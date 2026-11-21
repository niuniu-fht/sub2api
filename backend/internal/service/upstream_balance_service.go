package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/singleflight"
)

// 通用上游余额查询服务（对齐 new-api 渠道余额监控）。
//
// 覆盖两类可查询账号（均为 API-key 型上游）：
//  1. OpenAI 兼容中转站（new-api/one-api 系普遍实现，供 one 系互查）：
//     GET {base}/v1/dashboard/billing/subscription → hard_limit_usd（总额度 USD）
//     GET {base}/v1/dashboard/billing/usage?start_date&end_date → total_usage（单位 0.01 美元）
//     余额 = hard_limit_usd - total_usage/100；日期窗口：has_payment_method=true 取本月
//     1 号至今天，否则近 100 天（与 new-api controller/channel-billing.go 一致）。
//  2. Sub2API 站：GET {base}/v1/usage → remaining（优先）/balance，isValid==false 拒绝。
//     OpenAI 兼容端点 404/401 时回退探测该协议。
//
// 其余平台（gemini/claude oauth 等）无公开余额端点，返回"暂不支持"。
const (
	upstreamBalanceTimeout      = 30 * time.Second
	upstreamBalanceMaxBodyBytes = 64 * 1024
	// upstreamBalanceExtraKey 快照在 accounts.extra 下的键。
	upstreamBalanceExtraKey = "upstream_balance"

	// 批量刷新参数（对齐 new-api channel_balance.go）。
	upstreamBalanceConcurrency = 4
	upstreamBalanceInterval    = 500 * time.Millisecond
)

// UpstreamBalanceSnapshot 是 extra.upstream_balance 的持久化快照。
type UpstreamBalanceSnapshot struct {
	Status     string  `json:"status"` // success | error | unsupported
	Protocol   string  `json:"protocol,omitempty"`
	Balance    float64 `json:"balance,omitempty"`
	HardLimit  float64 `json:"hard_limit,omitempty"` // 总额度 USD
	Used       float64 `json:"used,omitempty"`       // 已用 USD
	Currency   string  `json:"currency,omitempty"`
	CheckedAt  string  `json:"checked_at,omitempty"` // RFC3339
	Error      string  `json:"error,omitempty"`
	HTTPStatus int     `json:"http_status,omitempty"`
}

// UpstreamBalanceResult 是单账号查询的即时返回（管理端 API 消费）。
type UpstreamBalanceResult struct {
	AccountID int64  `json:"account_id"`
	Status    string `json:"status"`
	Protocol  string `json:"protocol,omitempty"`
	Balance   float64 `json:"balance,omitempty"`
	HardLimit float64 `json:"hard_limit,omitempty"`
	Used      float64 `json:"used,omitempty"`
	Currency  string  `json:"currency,omitempty"`
	Error     string  `json:"error,omitempty"`
	Persisted bool    `json:"persisted"`
}

// UpstreamBalanceService 查询 API-key 型上游账号的账户余额并落 Extra 快照。
type UpstreamBalanceService struct {
	accountRepo  AccountRepository
	proxyRepo    ProxyRepository
	httpUpstream HTTPUpstream
	cfg          *config.Config
	flight       singleflight.Group
}

func NewUpstreamBalanceService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *UpstreamBalanceService {
	return &UpstreamBalanceService{
		accountRepo:  accountRepo,
		proxyRepo:    proxyRepo,
		httpUpstream: httpUpstream,
		cfg:          cfg,
	}
}

// QueryBalance 查询指定账号余额（singleflight 按 ID 合并并发），并落 Extra 快照。
func (s *UpstreamBalanceService) QueryBalance(ctx context.Context, accountID int64) (*UpstreamBalanceResult, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "UPSTREAM_BALANCE_NOT_CONFIGURED", "upstream balance service is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusNotFound, "UPSTREAM_BALANCE_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	key := "upstream_balance:" + strconv.FormatInt(account.ID, 10)
	resultCh := s.flight.DoChan(key, func() (any, error) {
		probeCtx, cancel := context.WithTimeout(context.Background(), upstreamBalanceTimeout+5*time.Second)
		defer cancel()
		return s.queryAccount(probeCtx, account)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case flightResult := <-resultCh:
		if flightResult.Err != nil {
			return nil, flightResult.Err
		}
		result, ok := flightResult.Val.(*UpstreamBalanceResult)
		if !ok || result == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "UPSTREAM_BALANCE_RESULT_INVALID", "invalid upstream balance result")
		}
		cloned := *result
		return &cloned, nil
	}
}

// RefreshAll 批量刷新：ids 为空时自动选择全部可查询账号（openai 平台 apikey 型）。
// 并发 upstreamBalanceConcurrency、账号间限速 upstreamBalanceInterval。
func (s *UpstreamBalanceService) RefreshAll(ctx context.Context, ids []int64) ([]*UpstreamBalanceResult, error) {
	targets, err := s.resolveTargets(ctx, ids)
	if err != nil {
		return nil, err
	}
	results := make([]*UpstreamBalanceResult, len(targets))
	sem := make(chan struct{}, upstreamBalanceConcurrency)
	var wg sync.WaitGroup
	var rateMu sync.Mutex
	lastStart := time.Now().Add(-upstreamBalanceInterval)
	for i, account := range targets {
		wg.Add(1)
		go func(idx int, acc Account) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// 简单全局限速，避免把上游打爆（与 new-api RequestInterval 同义）。
			rateMu.Lock()
			wait := upstreamBalanceInterval - time.Since(lastStart)
			if wait > 0 {
				time.Sleep(wait)
			}
			lastStart = time.Now()
			rateMu.Unlock()
			probeCtx, cancel := context.WithTimeout(context.Background(), upstreamBalanceTimeout)
			defer cancel()
			res, err := s.queryAccount(probeCtx, &acc)
			if err != nil || res == nil {
				res = &UpstreamBalanceResult{AccountID: acc.ID, Status: "error"}
				if err != nil {
					res.Error = err.Error()
				}
			}
			results[idx] = res
		}(i, *account)
	}
	wg.Wait()
	return results, nil
}

// ListBalances 返回全部账号的最新快照视图（管理端列表）。
func (s *UpstreamBalanceService) ListBalances(ctx context.Context) ([]UpstreamBalanceListItem, error) {
	accounts, _, err := s.accountRepo.List(ctx, paginationAllAccounts())
	if err != nil {
		return nil, err
	}
	items := make([]UpstreamBalanceListItem, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		item := UpstreamBalanceListItem{
			AccountID:     acc.ID,
			AccountName:   acc.Name,
			Platform:      acc.Platform,
			Type:          acc.Type,
			Schedulable:   acc.Schedulable,
			GroupIDs:      acc.GroupIDs,
			Queryable:     upstreamBalanceQueryable(acc),
		}
		if raw, ok := acc.Extra[upstreamBalanceExtraKey].(map[string]any); ok {
			item.Snapshot = extraMapToUpstreamBalanceSnapshot(raw)
		}
		if base := strings.TrimSpace(acc.GetOpenAIBaseURL()); base != "" {
			if u, err := url.Parse(base); err == nil {
				item.BaseURLHost = u.Host
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// UpstreamBalanceListItem 是管理端列表行：账号信息 + 最新余额快照。
type UpstreamBalanceListItem struct {
	AccountID    int64                    `json:"account_id"`
	AccountName  string                   `json:"account_name"`
	Platform     string                   `json:"platform"`
	Type         string                   `json:"type"`
	Schedulable  bool                     `json:"schedulable"`
	GroupIDs     []int64                  `json:"group_ids,omitempty"`
	BaseURLHost  string                   `json:"base_url_host,omitempty"`
	Queryable    bool                     `json:"queryable"`
	Snapshot     *UpstreamBalanceSnapshot `json:"snapshot,omitempty"`
}

// ─── 内部实现 ───

func (s *UpstreamBalanceService) resolveTargets(ctx context.Context, ids []int64) ([]*Account, error) {
	if len(ids) > 0 {
		targets := make([]*Account, 0, len(ids))
		for _, id := range ids {
			account, err := s.accountRepo.GetByID(ctx, id)
			if err != nil {
				continue
			}
			targets = append(targets, account)
		}
		return targets, nil
	}
	accounts, _, err := s.accountRepo.List(ctx, paginationAllAccounts())
	if err != nil {
		return nil, err
	}
	targets := make([]*Account, 0, len(accounts))
	for i := range accounts {
		if upstreamBalanceQueryable(&accounts[i]) {
			targets = append(targets, &accounts[i])
		}
	}
	return targets, nil
}

// upstreamBalanceQueryable 判断账号是否可查询余额：
// openai 平台 + apikey 型（自定义 base_url 的中转站/官方 key 均可）。
func upstreamBalanceQueryable(account *Account) bool {
	if account == nil {
		return false
	}
	return account.Platform == PlatformOpenAI &&
		(account.Type == AccountTypeAPIKey || account.Type == AccountTypeUpstream)
}

func (s *UpstreamBalanceService) queryAccount(ctx context.Context, account *Account) (*UpstreamBalanceResult, error) {
	now := time.Now().UTC()
	if !upstreamBalanceQueryable(account) {
		return s.persist(ctx, account.ID, &UpstreamBalanceResult{
			AccountID: account.ID,
			Status:    "unsupported",
			Error:     "platform or account type has no balance endpoint",
		}, &UpstreamBalanceSnapshot{
			Status:    "unsupported",
			CheckedAt: now.Format(time.RFC3339),
			Error:     "platform or account type has no balance endpoint",
		}, now)
	}

	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		return s.fail(ctx, account.ID, now, "account api_key is empty")
	}
	base := strings.TrimSpace(account.GetOpenAIBaseURL())
	if base == "" {
		return s.fail(ctx, account.ID, now, "account base_url is empty")
	}

	// 1) OpenAI 兼容双接口（new-api/one-api 系通用余额协议）。
	res, err := s.queryOpenAIBilling(ctx, account, base, apiKey)
	if err == nil {
		return s.persist(ctx, account.ID, res, snapshotFromResult(res, now), now)
	}
	openaiErr := err.Error()

	// 2) 回退：Sub2API 站 /v1/usage。
	res2, err2 := s.querySub2APIUsage(ctx, account, base, apiKey)
	if err2 == nil {
		return s.persist(ctx, account.ID, res2, snapshotFromResult(res2, now), now)
	}
	return s.fail(ctx, account.ID, now, fmt.Sprintf("openai billing: %s; sub2api usage: %s", openaiErr, err2.Error()))
}

// queryOpenAIBilling 两段式查询：subscription(hard_limit_usd) + usage(total_usage/100)。
func (s *UpstreamBalanceService) queryOpenAIBilling(ctx context.Context, account *Account, base, apiKey string) (*UpstreamBalanceResult, error) {
	subBody, subStatus, err := s.doBalanceGet(ctx, account, strings.TrimRight(base, "/")+"/v1/dashboard/billing/subscription", apiKey)
	if err != nil {
		return nil, err
	}
	if subStatus == http.StatusNotFound || subStatus == http.StatusUnauthorized || subStatus == http.StatusForbidden {
		return nil, fmt.Errorf("subscription endpoint returned HTTP %d", subStatus)
	}
	if subStatus < 200 || subStatus >= 300 {
		return nil, fmt.Errorf("subscription endpoint returned HTTP %d: %s", subStatus, truncateUpstreamBalanceText(string(subBody), 120))
	}
	return parseOpenAIBillingResult(account.ID, subBody, func(windowStart, windowEnd string) ([]byte, int, error) {
		usageURL := fmt.Sprintf("%s/v1/dashboard/billing/usage?start_date=%s&end_date=%s", strings.TrimRight(base, "/"), windowStart, windowEnd)
		return s.doBalanceGet(ctx, account, usageURL, apiKey)
	})
}

// upstreamBalanceUsageWindow 计算用量查询日期窗口（与 new-api 一致）：
// 有支付方式 → 本月 1 号至今天；否则近 100 天（总额度未绑定支付时的宽窗口）。
func upstreamBalanceUsageWindow(hasPaymentMethod bool, now time.Time) (string, string) {
	if hasPaymentMethod {
		return fmt.Sprintf("%s-01", now.Format("2006-01")), now.Format("2006-01-02")
	}
	return now.AddDate(0, 0, -100).Format("2006-01-02"), now.Format("2006-01-02")
}

// parseOpenAIBillingResult 解析 subscription 响应并按窗口拉取 usage，
// 余额 = hard_limit_usd - total_usage/100（total_usage 单位 0.01 美元）。
func parseOpenAIBillingResult(accountID int64, subBody []byte, fetchUsage func(start, end string) ([]byte, int, error)) (*UpstreamBalanceResult, error) {
	hardLimit := gjson.GetBytes(subBody, "hard_limit_usd").Float()
	hasPaymentMethod := gjson.GetBytes(subBody, "has_payment_method").Bool()

	startDate, endDate := upstreamBalanceUsageWindow(hasPaymentMethod, time.Now())
	usageBody, usageStatus, err := fetchUsage(startDate, endDate)
	if err != nil {
		return nil, err
	}
	if usageStatus < 200 || usageStatus >= 300 {
		return nil, fmt.Errorf("usage endpoint returned HTTP %d: %s", usageStatus, truncateUpstreamBalanceText(string(usageBody), 120))
	}
	// total_usage 单位是 0.01 美元。
	used := gjson.GetBytes(usageBody, "total_usage").Float() / 100
	return &UpstreamBalanceResult{
		AccountID: accountID,
		Status:    "success",
		Protocol:  "openai_billing",
		Balance:   hardLimit - used,
		HardLimit: hardLimit,
		Used:      used,
		Currency:  "USD",
	}, nil
}

// querySub2APIUsage 查询 sub2api 系站点的 GET {base}/v1/usage。
func (s *UpstreamBalanceService) querySub2APIUsage(ctx context.Context, account *Account, base, apiKey string) (*UpstreamBalanceResult, error) {
	body, status, err := s.doBalanceGet(ctx, account, strings.TrimRight(base, "/")+"/v1/usage", apiKey)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound || status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, fmt.Errorf("/v1/usage returned HTTP %d", status)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("/v1/usage returned HTTP %d: %s", status, truncateUpstreamBalanceText(string(body), 120))
	}
	return parseSub2APIUsageResult(account.ID, body)
}

// parseSub2APIUsageResult 解析 sub2api 站 /v1/usage 响应：
// remaining 优先于 balance；isValid 显式为 false 时拒绝（与 new-api Sub2API 分支一致）。
func parseSub2APIUsageResult(accountID int64, body []byte) (*UpstreamBalanceResult, error) {
	if v := gjson.GetBytes(body, "isValid"); v.Exists() && !v.Bool() {
		return nil, fmt.Errorf("upstream reports isValid=false")
	}
	remaining := gjson.GetBytes(body, "remaining")
	if !remaining.Exists() {
		remaining = gjson.GetBytes(body, "balance")
	}
	if !remaining.Exists() {
		return nil, fmt.Errorf("response has no remaining/balance field")
	}
	return &UpstreamBalanceResult{
		AccountID: accountID,
		Status:    "success",
		Protocol:  "sub2api_usage",
		Balance:   remaining.Float(),
		Currency:  "USD",
	}, nil
}

func (s *UpstreamBalanceService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	if s != nil && s.proxyRepo != nil {
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			account.Proxy = proxy
			return proxy.URL()
		}
	}
	return ""
}

func (s *UpstreamBalanceService) doBalanceGet(ctx context.Context, account *Account, targetURL, apiKey string) ([]byte, int, error) {
	validated, err := cnValidateProbeURL(s.cfg, targetURL)
	if err != nil {
		return nil, 0, err
	}
	proxyURL := s.resolveProxyURL(ctx, account)
	callCtx, cancel := context.WithTimeout(ctx, upstreamBalanceTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, validated, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(req.Header)

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return nil, 0, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, upstreamBalanceMaxBodyBytes))
	return body, resp.StatusCode, nil
}

func (s *UpstreamBalanceService) fail(ctx context.Context, accountID int64, now time.Time, msg string) (*UpstreamBalanceResult, error) {
	return s.persist(ctx, accountID,
		&UpstreamBalanceResult{AccountID: accountID, Status: "error", Error: msg},
		&UpstreamBalanceSnapshot{Status: "error", CheckedAt: now.Format(time.RFC3339), Error: msg},
		now)
}

func (s *UpstreamBalanceService) persist(ctx context.Context, accountID int64, result *UpstreamBalanceResult, snapshot *UpstreamBalanceSnapshot, now time.Time) (*UpstreamBalanceResult, error) {
	if err := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
		upstreamBalanceExtraKey: map[string]any{
			"status":      snapshot.Status,
			"protocol":    snapshot.Protocol,
			"balance":     snapshot.Balance,
			"hard_limit":  snapshot.HardLimit,
			"used":        snapshot.Used,
			"currency":    snapshot.Currency,
			"checked_at":  now.Format(time.RFC3339),
			"error":       truncateUpstreamBalanceText(snapshot.Error, 300),
			"http_status": snapshot.HTTPStatus,
		},
	}); err != nil {
		slog.Warn("upstream_balance_persist_failed", "account_id", accountID, "error", err)
	} else {
		result.Persisted = true
	}
	return result, nil
}

func snapshotFromResult(res *UpstreamBalanceResult, now time.Time) *UpstreamBalanceSnapshot {
	return &UpstreamBalanceSnapshot{
		Status:    res.Status,
		Protocol:  res.Protocol,
		Balance:   res.Balance,
		HardLimit: res.HardLimit,
		Used:      res.Used,
		Currency:  res.Currency,
		CheckedAt: now.Format(time.RFC3339),
	}
}

// extraMapToUpstreamBalanceSnapshot 从 accounts.extra 的 JSON map 还原快照。
func extraMapToUpstreamBalanceSnapshot(raw map[string]any) *UpstreamBalanceSnapshot {
	if raw == nil {
		return nil
	}
	getStr := func(k string) string { v, _ := raw[k].(string); return v }
	getF64 := func(k string) float64 {
		switch v := raw[k].(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		default:
			return 0
		}
	}
	getInt := func(k string) int {
		switch v := raw[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		default:
			return 0
		}
	}
	return &UpstreamBalanceSnapshot{
		Status:     getStr("status"),
		Protocol:   getStr("protocol"),
		Balance:    getF64("balance"),
		HardLimit:  getF64("hard_limit"),
		Used:       getF64("used"),
		Currency:   getStr("currency"),
		CheckedAt:  getStr("checked_at"),
		Error:      getStr("error"),
		HTTPStatus: getInt("http_status"),
	}
}

func truncateUpstreamBalanceText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// paginationAllAccounts 构造"取全部账号"的分页参数（上限对齐管理端列表页）。
func paginationAllAccounts() pagination.PaginationParams {
	return pagination.PaginationParams{Page: 1, PageSize: 1000}
}
