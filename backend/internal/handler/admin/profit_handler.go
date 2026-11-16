package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/repository"

	"github.com/gin-gonic/gin"
)

// ProfitHandler 利润统计接口（仅管理员，挂在 adminAuth 组内）。
//
// 口径：收入 = 用户消费(actual_cost) × 用户利润折扣率(覆盖或全局默认)；
// 成本 = usage_logs.account_stats_cost（渠道"账号统计定价"每请求快照，
// 未配置时按 total_cost × account_rate_multiplier 估算并在响应中标注占比）。
// 只读聚合 + 费率/默认率维护，不触碰计费链路。
type ProfitHandler struct {
	repo *repository.ProfitRepository
}

func NewProfitHandler(repo *repository.ProfitRepository) *ProfitHandler {
	return &ProfitHandler{repo: repo}
}

// parseProfitRange 解析 start/end：支持 YYYY-MM-DD 与 RFC3339；
// 缺省 end=now、start=end-24h。
func parseProfitRange(c *gin.Context) (time.Time, time.Time, bool) {
	end := time.Now()
	start := end.AddDate(0, 0, -7)
	if s := c.Query("start"); s != "" {
		t, err := parseProfitTime(s)
		if err != nil {
			response.BadRequest(c, "Invalid start: "+err.Error())
			return time.Time{}, time.Time{}, false
		}
		start = t
	}
	if e := c.Query("end"); e != "" {
		t, err := parseProfitTime(e, true)
		if err != nil {
			response.BadRequest(c, "Invalid end: "+err.Error())
			return time.Time{}, time.Time{}, false
		}
		end = t
	}
	if !start.Before(end) {
		response.BadRequest(c, "start must be before end")
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

func parseProfitTime(s string, endOfDay ...bool) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			if len(endOfDay) > 0 && endOfDay[0] && layout == "2006-01-02" {
				return t.AddDate(0, 0, 1), nil // end 含当天全天
			}
			return t, nil
		}
	}
	return time.Time{}, errProfitTime(s)
}

type profitTimeError struct{ s string }

func (e *profitTimeError) Error() string { return "invalid time format (expect YYYY-MM-DD or RFC3339): " + e.s }
func errProfitTime(s string) error      { return &profitTimeError{s} }

// Summary GET /api/v1/admin/profit/summary?start&end
func (h *ProfitHandler) Summary(c *gin.Context) {
	start, end, ok := parseProfitRange(c)
	if !ok {
		return
	}
	summary, err := h.repo.Summary(c.Request.Context(), start, end)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"start": start.Format(time.RFC3339), "end": end.Format(time.RFC3339),
		"totals": summary.Totals, "daily": summary.Daily,
	})
}

// ListUsers GET /api/v1/admin/profit/users?start&end&search&order&page&page_size
func (h *ProfitHandler) ListUsers(c *gin.Context) {
	start, end, ok := parseProfitRange(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize > 200 {
		pageSize = 200
	}
	rows, total, err := h.repo.ByUsers(c.Request.Context(), start, end, c.Query("search"), c.Query("order"), pageSize, (page-1)*pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		items = append(items, gin.H{
			"user_id": r.UserID, "email": r.Email, "name": r.Name,
			"rate": r.Rate, "rate_set": r.RateSet,
			"requests": r.Requests, "images": r.Images,
			"revenue": r.Revenue, "cost": r.Cost,
			"profit": r.Profit(),
			"margin": marginOf(r.Revenue, r.Profit()),
		})
	}
	response.Success(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func marginOf(revenue, profit float64) float64 {
	if revenue <= 0 {
		return 0
	}
	return profit / revenue
}

// UserDetail GET /api/v1/admin/profit/users/:id?start&end
func (h *ProfitHandler) UserDetail(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	start, end, ok := parseProfitRange(c)
	if !ok {
		return
	}
	detail, err := h.repo.UserDetail(c.Request.Context(), userID, start, end)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	models := make([]gin.H, 0, len(detail.ByModel))
	for _, m := range detail.ByModel {
		models = append(models, gin.H{
			"model": m.Model, "platform": m.Platform, "requests": m.Requests, "images": m.Images,
			"gross_revenue": m.GrossRev, "revenue": m.Revenue, "cost": m.Cost,
			"profit": m.Profit(), "margin": marginOf(m.Revenue, m.Profit()),
		})
	}
	accounts := make([]gin.H, 0, len(detail.ByAccount))
	for _, a := range detail.ByAccount {
		accounts = append(accounts, gin.H{
			"account_id": a.AccountID, "account_name": a.AccountName, "platform": a.Platform,
			"requests": a.Requests, "images": a.Images,
			"revenue": a.Revenue, "cost": a.Cost,
			"profit": a.Profit(), "margin": marginOf(a.Revenue, a.Profit()),
		})
	}
	response.Success(c, gin.H{
		"user_id": userID, "rate": detail.Rate, "rate_set": detail.RateSet,
		"totals": detail.Totals, "by_model": models, "by_account": accounts, "daily": detail.Daily,
	})
}

// ListModels GET /api/v1/admin/profit/models?start&end
func (h *ProfitHandler) ListModels(c *gin.Context) {
	start, end, ok := parseProfitRange(c)
	if !ok {
		return
	}
	rows, err := h.repo.ByModels(c.Request.Context(), start, end)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, m := range rows {
		items = append(items, gin.H{
			"model": m.Model, "platform": m.Platform, "requests": m.Requests, "images": m.Images,
			"gross_revenue": m.GrossRev, "revenue": m.Revenue, "cost": m.Cost,
			"profit": m.Profit(), "margin": marginOf(m.Revenue, m.Profit()),
		})
	}
	response.Success(c, gin.H{"items": items})
}

// ListAccounts GET /api/v1/admin/profit/accounts?start&end
func (h *ProfitHandler) ListAccounts(c *gin.Context) {
	start, end, ok := parseProfitRange(c)
	if !ok {
		return
	}
	rows, err := h.repo.ByAccounts(c.Request.Context(), start, end, nil)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, a := range rows {
		items = append(items, gin.H{
			"account_id": a.AccountID, "account_name": a.AccountName, "platform": a.Platform,
			"requests": a.Requests, "images": a.Images,
			"revenue": a.Revenue, "cost": a.Cost,
			"profit": a.Profit(), "margin": marginOf(a.Revenue, a.Profit()),
		})
	}
	response.Success(c, gin.H{"items": items})
}

// ListRates GET /api/v1/admin/profit/rates
func (h *ProfitHandler) ListRates(c *gin.Context) {
	rows, err := h.repo.ListUserRates(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": rows})
}

type updateProfitRateRequest struct {
	Rate float64 `json:"rate" binding:"required,gt=0,lte=10"`
	Note string  `json:"note"`
}

// UpsertRate PUT /api/v1/admin/profit/rates/:userId
func (h *ProfitHandler) UpsertRate(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("userId"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	var req updateProfitRateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.repo.UpsertUserRate(c.Request.Context(), userID, req.Rate, req.Note); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"user_id": userID, "rate": req.Rate})
}

// DeleteRate DELETE /api/v1/admin/profit/rates/:userId
func (h *ProfitHandler) DeleteRate(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("userId"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	if err := h.repo.DeleteUserRate(c.Request.Context(), userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"user_id": userID})
}

// GetConfig GET /api/v1/admin/profit/config
func (h *ProfitHandler) GetConfig(c *gin.Context) {
	cfg, err := h.repo.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

type updateProfitConfigRequest struct {
	DefaultUserRate float64 `json:"default_user_rate" binding:"required,gt=0,lte=10"`
}

// UpdateConfig PUT /api/v1/admin/profit/config
func (h *ProfitHandler) UpdateConfig(c *gin.Context) {
	var req updateProfitConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.repo.SaveConfig(c.Request.Context(), repository.ProfitConfig{DefaultUserRate: req.DefaultUserRate}); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, req)
}

// ─── 成本规则（利润模块私有，与渠道定价无关）───


// CostRuleModelOptions GET /api/v1/admin/profit/cost-rules/models?account_id=
// 返回该账号（可空=全局）可选模型：映射键/值 + 近30天实际调用模型。
func (h *ProfitHandler) CostRuleModelOptions(c *gin.Context) {
	var accountID *int64
	if raw := c.Query("account_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid account_id")
			return
		}
		accountID = &id
	}
	models, err := h.repo.CostRuleModelOptions(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": models})
}


type replaceCostRulesRequest struct {
	AccountID *int64                  `json:"account_id"`
	Rules     []costRuleRequestRow `json:"rules"`
}

type costRuleRequestRow struct {
	Model        string  `json:"model"`
	BillingMode  string  `json:"billing_mode" binding:"required,oneof=token per_image per_request"`
	InputPrice   float64 `json:"input_price"`
	OutputPrice  float64 `json:"output_price"`
	ImagePrice   float64 `json:"image_price"`
	RequestPrice float64 `json:"request_price"`
	Enabled      *bool   `json:"enabled"`
	Note         string  `json:"note"`
}

// ReplaceCostRules POST /api/v1/admin/profit/cost-rules/batch
// 批量保存某账号范围的全部成本规则；价格为 0/未填的模型不生成规则（成本按 0 计）。
func (h *ProfitHandler) ReplaceCostRules(c *gin.Context) {
	var req replaceCostRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	rules := make([]repository.ProfitCostRule, 0, len(req.Rules))
	for _, row := range req.Rules {
		if strings.TrimSpace(row.Model) == "" {
			response.BadRequest(c, "rule model must not be empty")
			return
		}
		enabled := true
		if row.Enabled != nil {
			enabled = *row.Enabled
		}
		rules = append(rules, repository.ProfitCostRule{
			AccountID:    req.AccountID,
			Model:        strings.TrimSpace(row.Model),
			BillingMode:  row.BillingMode,
			InputPrice:   row.InputPrice,
			OutputPrice:  row.OutputPrice,
			ImagePrice:   row.ImagePrice,
			RequestPrice: row.RequestPrice,
			Enabled:      enabled,
			Note:         row.Note,
		})
	}
	if err := h.repo.ReplaceCostRules(c.Request.Context(), req.AccountID, rules); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"saved": len(rules)})
}

// ListCostRules GET /api/v1/admin/profit/cost-rules
func (h *ProfitHandler) ListCostRules(c *gin.Context) {
	rules, err := h.repo.ListCostRules(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": rules})
}

type costRuleRequest struct {
	AccountID    *int64  `json:"account_id"` // nil = 任意账号
	Model        string  `json:"model"`      // '' = 任意模型
	BillingMode  string  `json:"billing_mode" binding:"required,oneof=token per_image per_request"`
	InputPrice   float64 `json:"input_price"`
	OutputPrice  float64 `json:"output_price"`
	ImagePrice   float64 `json:"image_price"`
	RequestPrice float64 `json:"request_price"`
	Enabled      *bool   `json:"enabled"`
	Note         string  `json:"note"`
}

func (req *costRuleRequest) toRule() repository.ProfitCostRule {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return repository.ProfitCostRule{
		AccountID:    req.AccountID,
		Model:        strings.TrimSpace(req.Model),
		BillingMode:  req.BillingMode,
		InputPrice:   req.InputPrice,
		OutputPrice:  req.OutputPrice,
		ImagePrice:   req.ImagePrice,
		RequestPrice: req.RequestPrice,
		Enabled:      enabled,
		Note:         req.Note,
	}
}

// CreateCostRule POST /api/v1/admin/profit/cost-rules
func (h *ProfitHandler) CreateCostRule(c *gin.Context) {
	var req costRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	rule := req.toRule()
	if rule.BillingMode == "per_image" && rule.ImagePrice <= 0 {
		response.BadRequest(c, "image_price must be > 0 for per_image mode")
		return
	}
	if rule.BillingMode == "per_request" && rule.RequestPrice <= 0 {
		response.BadRequest(c, "request_price must be > 0 for per_request mode")
		return
	}
	if rule.BillingMode == "token" && rule.InputPrice <= 0 && rule.OutputPrice <= 0 {
		response.BadRequest(c, "input_price/output_price must be > 0 for token mode")
		return
	}
	id, err := h.repo.CreateCostRule(c.Request.Context(), rule)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

// UpdateCostRule PUT /api/v1/admin/profit/cost-rules/:id
func (h *ProfitHandler) UpdateCostRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
	}
	var req costRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.repo.UpdateCostRule(c.Request.Context(), id, req.toRule()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

// DeleteCostRule DELETE /api/v1/admin/profit/cost-rules/:id
func (h *ProfitHandler) DeleteCostRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
	}
	if err := h.repo.DeleteCostRule(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}
