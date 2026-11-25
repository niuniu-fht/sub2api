package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/redis/go-redis/v9"
)

// ChannelStatusService 渠道状态统计服务：
// 定时聚合真实生成记录（usage_logs 成功请求 / ops_error_logs 失败记录），
// 按「分组 × 模型」计算成功率与平均生图时间，结果刷入
// Redis + 进程内存缓存，对外公开 API（GET /api/v1/channel-status）只读缓存。
// 完全被动聚合，不发起任何探测请求；与 channel_monitor v1/v2 相互独立。
type ChannelStatusService struct {
	settingService *SettingService
	db             *sql.DB
	redisClient    *redis.Client

	mu       sync.RWMutex
	snapshot *ChannelStatusSnapshot

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

const channelStatusSnapshotRedisKey = "channel_status:snapshot"

// recoveredUpstreamStatusRe 匹配「已恢复的中间失败」消息里的真实上游状态码，
// 如 "Recovered upstream error 502: Image generation timed out"。
var recoveredUpstreamStatusRe = regexp.MustCompile(`Recovered upstream error (\d+)`)

// msgEmbeddedHTTPStatusRe 提取消息内嵌的真实 HTTP 状态码（如 "返回 HTTP 429、"）。
var msgEmbeddedHTTPStatusRe = regexp.MustCompile(`HTTP (\d{3})`)

// classifyStatus200 对 status_code=200 的 ops 行细分：
//   - "Recovered upstream error N" → 已恢复的中间失败，分类 "↻N"，计入尝试失败；
//   - 其他 "Recovered …"（传输层错误/账号鉴权失败恢复，无状态码）→ "↻net"；
//   - "<badge color="success">" 开头 → 成功行误入错误表，完全跳过；
//   - 消息内嵌 "HTTP N"（如 AI Studio 429 包在 200 行）→ 分类真实码 "N"，计入失败；
//   - 其余（池不可用/无可用账号/分组停用等业务失败）→ 分类 "200"，计入失败。
//
// skip=true 表示该行既不算失败也不进分类（用户视角与尝试视角一致跳过）。
func classifyStatus200(message string) (classStatus string, skip bool) {
	if strings.HasPrefix(message, "<badge color=\"success\">") {
		return "", true
	}
	if m := recoveredUpstreamStatusRe.FindStringSubmatch(message); m != nil {
		return "↻" + m[1], false
	}
	if strings.HasPrefix(message, "Recovered ") {
		return "↻net", false
	}
	if m := msgEmbeddedHTTPStatusRe.FindStringSubmatch(message); m != nil {
		return m[1], false
	}
	return "200", false
}

// isRecoveredStatus200Row 判断 status_code=200 的 ops 行是否为「已恢复的中间失败」：
// 客户端最终成功（200），ops 行只记录换号前某次尝试的失败供账号层面观测。
// 覆盖 "Recovered upstream error N: …"、无状态码的传输层恢复
// （"Recovered upstream error: Post …: dial tcp …"）与账号鉴权失败恢复。
func isRecoveredStatus200Row(message string) bool {
	return strings.HasPrefix(message, "Recovered ")
}

// ---- 对外数据结构（JSON 即 API 响应契约，字段只含可公开展示信息） ----

type ChannelStatusModelStatus struct {
	Model       string  `json:"model"`
	Requests    int64   `json:"requests"`
	Failures    int64   `json:"failures"`
	SuccessRate float64 `json:"success_rate"` // 百分比 0-100；无请求时 -1 表示 unknown
	AvgGenMs    int64   `json:"avg_gen_ms"`   // 成功请求平均耗时；0 表示无样本
	FastestMs   int64   `json:"fastest_ms"`   // 最快成功耗时；0 表示无样本
	SlowestMs   int64   `json:"slowest_ms"`   // 最长成功耗时
	P95Ms       int64   `json:"p95_ms"`       // 成功耗时 95 分位（percentile_cont 插值）
	P99Ms       int64   `json:"p99_ms"`       // 成功耗时 99 分位
}

type ChannelStatusBucket struct {
	BucketStartMs int64   `json:"bucket_start_ms"`
	Requests      int64   `json:"requests"`
	Failures      int64   `json:"failures"`
	SuccessRate   float64 `json:"success_rate"`
	Status        string  `json:"status"` // operational | degraded | down | unknown
	// 该小时桶内的按模型明细（成功率/平均耗时只统计本桶），供前端点击时间线
	// 下钻展示；桶内无请求时为空。
	Models []ChannelStatusModelStatus `json:"models,omitempty"`
}

// ChannelStatusLastHour 最近 1 小时的滚动口径统计（成功率/平均耗时）。
// 用户态状态页默认展示该口径，完整窗口数据可切换查看；详细分布（最快/
// 最长/P95/P99）仅管理端接口返回，公开接口会脱敏掉。
type ChannelStatusLastHour struct {
	Requests    int64   `json:"requests"`
	Failures    int64   `json:"failures"`
	SuccessRate float64 `json:"success_rate"` // -1 表示该小时无请求
	AvgGenMs    int64   `json:"avg_gen_ms"`
}

type GroupStatus struct {
	ID          int64                      `json:"id"`
	Name        string                     `json:"name"`
	Platform    string                     `json:"platform"`
	Status      string                     `json:"status"` // operational | degraded | down | unknown
	Requests    int64                      `json:"requests"`
	Failures    int64                      `json:"failures"`
	SuccessRate float64                    `json:"success_rate"`
	AvgGenMs    int64                      `json:"avg_gen_ms"`
	FastestMs   int64                      `json:"fastest_ms"` // 分组级成功耗时分布
	SlowestMs   int64                      `json:"slowest_ms"`
	P95Ms       int64                      `json:"p95_ms"`
	P99Ms       int64                      `json:"p99_ms"`
	LastHour    *ChannelStatusLastHour     `json:"last_hour,omitempty"`
	Models      []ChannelStatusModelStatus `json:"models"`
	Timeline    []ChannelStatusBucket      `json:"timeline"`
}

type ChannelStatusSnapshot struct {
	GeneratedAtMs int64  `json:"generated_at_ms"`
	WindowHours   int    `json:"window_hours"`
	RefreshSec    int    `json:"refresh_seconds"`
	Overall       string `json:"overall"` // operational | degraded | down | unknown
	// Thresholds 是状态分级阈值（healthy/degraded 百分比），随快照下发：
	// 前端切换「最近 1 小时」口径时用它对 last_hour 成功率重新分级状态徽章。
	Thresholds ChannelStatusThresholds `json:"thresholds"`
	AvgGenMs   int64                   `json:"avg_gen_ms"` // 全站成功请求平均耗时；0 表示无样本
	FastestMs  int64                   `json:"fastest_ms"` // 全站成功耗时分布
	SlowestMs  int64                   `json:"slowest_ms"`
	P95Ms      int64                   `json:"p95_ms"`
	P99Ms      int64                   `json:"p99_ms"`
	Groups     []GroupStatus           `json:"groups"`
}

const channelStatusTimelineBuckets = 24

// channelStatusGroupMeta 是组装快照时用到的分组元信息。
type channelStatusGroupMeta struct {
	Name      string
	Platform  string
	Exclusive bool
}

// channelStatusDisplayIDs 从窗口内有记录的分组（activeIDs）算出快照要输出的分组：
// 只保留公开分组（专属分组与已删除分组不进快照）；配置了展示白名单时只输出
// 白名单内的分组，并把白名单里窗口内没有记录的分组补在末尾（状态为 unknown）——
// 管理员显式勾选的分组必须可见，否则低流量分组勾选后看不到，会被误认为配置未生效。
// 白名单为空表示展示全部有记录的公开分组。
func channelStatusDisplayIDs(activeIDs, whitelist []int64, groupMeta map[int64]channelStatusGroupMeta) []int64 {
	isPublic := func(id int64) bool {
		meta, ok := groupMeta[id]
		return ok && !meta.Exclusive
	}
	if len(whitelist) == 0 {
		publicIDs := make([]int64, 0, len(activeIDs))
		for _, id := range activeIDs {
			if isPublic(id) {
				publicIDs = append(publicIDs, id)
			}
		}
		return publicIDs
	}
	allowed := make(map[int64]struct{}, len(whitelist))
	for _, id := range whitelist {
		allowed[id] = struct{}{}
	}
	displayIDs := make([]int64, 0, len(whitelist))
	seen := make(map[int64]struct{}, len(whitelist))
	for _, id := range activeIDs {
		if _, ok := allowed[id]; !ok || !isPublic(id) {
			continue
		}
		seen[id] = struct{}{}
		displayIDs = append(displayIDs, id)
	}
	for _, id := range whitelist {
		if _, ok := seen[id]; ok || !isPublic(id) {
			continue
		}
		seen[id] = struct{}{}
		displayIDs = append(displayIDs, id)
	}
	return displayIDs
}

// channelLatencyStats 成功请求耗时的分布统计（同一窗口内），由
// Aggregate/AccountSuccessStatsRange 的 percentile_cont 查询填充。
type channelLatencyStats struct {
	AvgMs     int64
	FastestMs int64
	SlowestMs int64
	P95Ms     int64
	P99Ms     int64
}

// classifyChannelStatus 按阈值把成功率（百分比）映射为状态。
func classifyChannelStatus(ratePct float64, hasTraffic bool, thresholds ChannelStatusThresholds) string {
	if !hasTraffic {
		return "unknown"
	}
	if ratePct >= thresholds.Healthy {
		return "operational"
	}
	if ratePct >= thresholds.Degraded {
		return "degraded"
	}
	return "down"
}

func ratePercent(success, total int64) float64 {
	if total <= 0 {
		return -1 // unknown
	}
	return math.Round(float64(success)/float64(total)*10000) / 100
}

// ---- 聚合中间结构 ----

type channelStatusAgg struct {
	successRequests map[int64]map[string]int64
	successSumMs    map[int64]map[string]float64
	successCntMs    map[int64]map[string]int64
	failures        map[int64]map[string]int64

	bucketStart    time.Time
	bucketSeconds  float64
	bucketRequests map[int64][]int64
	bucketFailures map[int64][]int64
	// 每桶 × 模型的成功/失败计数与成功耗时样本，支撑「点击时间线桶查看该小时
	// 模型成功率」。桶宽固定 24 个；内层 map 按需创建。
	bucketModelRequests map[int64][]map[string]int64
	bucketModelFailures map[int64][]map[string]int64
	bucketModelSumMs    map[int64][]map[string]float64
	bucketModelCntMs    map[int64][]map[string]int64
}

func newChannelStatusAgg(start time.Time, windowHours int) *channelStatusAgg {
	return &channelStatusAgg{
		successRequests:     map[int64]map[string]int64{},
		successSumMs:        map[int64]map[string]float64{},
		successCntMs:        map[int64]map[string]int64{},
		failures:            map[int64]map[string]int64{},
		bucketStart:         start,
		bucketSeconds:       float64(windowHours*3600) / float64(channelStatusTimelineBuckets),
		bucketRequests:      map[int64][]int64{},
		bucketFailures:      map[int64][]int64{},
		bucketModelRequests: map[int64][]map[string]int64{},
		bucketModelFailures: map[int64][]map[string]int64{},
		bucketModelSumMs:    map[int64][]map[string]float64{},
		bucketModelCntMs:    map[int64][]map[string]int64{},
	}
}

func (a *channelStatusAgg) ensureBucket(accountID int64) {
	if a.bucketRequests[accountID] == nil {
		a.bucketRequests[accountID] = make([]int64, channelStatusTimelineBuckets)
		a.bucketFailures[accountID] = make([]int64, channelStatusTimelineBuckets)
		a.bucketModelRequests[accountID] = make([]map[string]int64, channelStatusTimelineBuckets)
		a.bucketModelFailures[accountID] = make([]map[string]int64, channelStatusTimelineBuckets)
		a.bucketModelSumMs[accountID] = make([]map[string]float64, channelStatusTimelineBuckets)
		a.bucketModelCntMs[accountID] = make([]map[string]int64, channelStatusTimelineBuckets)
	}
}

// bucketModelMaps 返回指定账号指定桶的模型统计 map（按需初始化）。
func (a *channelStatusAgg) bucketModelMaps(accountID int64, idx int) (requests, failures map[string]int64, sumMs map[string]float64, cntMs map[string]int64) {
	requests = a.bucketModelRequests[accountID][idx]
	if requests == nil {
		requests = map[string]int64{}
		a.bucketModelRequests[accountID][idx] = requests
	}
	failures = a.bucketModelFailures[accountID][idx]
	if failures == nil {
		failures = map[string]int64{}
		a.bucketModelFailures[accountID][idx] = failures
	}
	sumMs = a.bucketModelSumMs[accountID][idx]
	if sumMs == nil {
		sumMs = map[string]float64{}
		a.bucketModelSumMs[accountID][idx] = sumMs
	}
	cntMs = a.bucketModelCntMs[accountID][idx]
	if cntMs == nil {
		cntMs = map[string]int64{}
		a.bucketModelCntMs[accountID][idx] = cntMs
	}
	return requests, failures, sumMs, cntMs
}

// bucketModels 汇总指定桶的模型明细（成功率/平均耗时按该桶数据计算，
// 只含有请求的模型，按请求量降序），供时间线桶的下钻展示。
// latency 是该分组每桶每模型的耗时分布（可为 nil，单元测试用）。
func (a *channelStatusAgg) bucketModels(accountID int64, idx int, latency map[int]map[string]channelLatencyStats) []ChannelStatusModelStatus {
	if a.bucketModelRequests[accountID] == nil || idx < 0 || idx >= channelStatusTimelineBuckets {
		return nil
	}
	requests := a.bucketModelRequests[accountID][idx]
	failures := a.bucketModelFailures[accountID][idx]
	sumMs := a.bucketModelSumMs[accountID][idx]
	cntMs := a.bucketModelCntMs[accountID][idx]
	models := make([]ChannelStatusModelStatus, 0, len(requests)+len(failures))
	modelSet := map[string]struct{}{}
	for m := range requests {
		modelSet[m] = struct{}{}
	}
	for m := range failures {
		modelSet[m] = struct{}{}
	}
	var bucketLatency map[string]channelLatencyStats
	if latency != nil {
		bucketLatency = latency[idx]
	}
	for m := range modelSet {
		reqN := requests[m]
		failN := failures[m]
		total := reqN + failN
		var avgMs int64
		if cntMs != nil && cntMs[m] > 0 {
			avgMs = int64(math.Round(sumMs[m] / float64(cntMs[m])))
		}
		ml := bucketLatency[m]
		models = append(models, ChannelStatusModelStatus{
			Model:       m,
			Requests:    reqN,
			Failures:    failN,
			SuccessRate: ratePercent(reqN, total),
			AvgGenMs:    avgMs,
			FastestMs:   ml.FastestMs,
			SlowestMs:   ml.SlowestMs,
			P95Ms:       ml.P95Ms,
			P99Ms:       ml.P99Ms,
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Requests > models[j].Requests })
	return models
}

func (a *channelStatusAgg) bucketIndexAt(t time.Time) int {
	idx := int(t.Sub(a.bucketStart).Seconds() / a.bucketSeconds)
	if idx < 0 {
		return 0
	}
	if idx >= channelStatusTimelineBuckets {
		return channelStatusTimelineBuckets - 1
	}
	return idx
}

// addSuccess 累计一组成功聚合行:requests 是 SQL GROUP BY 合并后的真实请求数
// (count(*)),durationMs 是该组的平均耗时(avg(duration_ms));两者都必须使用,
// 逐行 ++ 会把整组 N 次成功只计成 1 次,导致成功率被严重低估。
func (a *channelStatusAgg) addSuccess(accountID int64, model string, requests int64, durationMs int64, at time.Time) {
	if a.successRequests[accountID] == nil {
		a.successRequests[accountID] = map[string]int64{}
		a.successSumMs[accountID] = map[string]float64{}
		a.successCntMs[accountID] = map[string]int64{}
	}
	a.successRequests[accountID][model] += requests
	if durationMs > 0 {
		a.successSumMs[accountID][model] += float64(durationMs) * float64(requests)
		a.successCntMs[accountID][model] += requests
	}
	a.ensureBucket(accountID)
	idx := a.bucketIndexAt(at)
	a.bucketRequests[accountID][idx] += requests
	bReq, _, bSumMs, bCntMs := a.bucketModelMaps(accountID, idx)
	bReq[model] += requests
	if durationMs > 0 {
		bSumMs[model] += float64(durationMs) * float64(requests)
		bCntMs[model] += requests
	}
}

func (a *channelStatusAgg) addFailure(accountID int64, model string, at time.Time) {
	if a.failures[accountID] == nil {
		a.failures[accountID] = map[string]int64{}
	}
	a.failures[accountID][model]++
	a.ensureBucket(accountID)
	idx := a.bucketIndexAt(at)
	a.bucketFailures[accountID][idx]++
	_, bFailures, _, _ := a.bucketModelMaps(accountID, idx)
	bFailures[model]++
}

// ---- 服务实现 ----

func NewChannelStatusService(settingService *SettingService, db *sql.DB, redisClient *redis.Client) *ChannelStatusService {
	return &ChannelStatusService{
		settingService: settingService,
		db:             db,
		redisClient:    redisClient,
		stopCh:         make(chan struct{}),
	}
}

// Start 启动后台聚合循环（幂等）。
func (s *ChannelStatusService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		go func() {
			// 启动即跑一轮，让 API 尽快有数据
			s.runOnceSafe()
			interval := s.currentRefreshInterval()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-s.stopCh:
					return
				case <-ticker.C:
					s.runOnceSafe()
					// 配置热更新：refresh_seconds 变化时重建 ticker
					if want := s.currentRefreshInterval(); want != interval {
						interval = want
						ticker.Reset(interval)
					}
				}
			}
		}()
	})
}

// Stop 停止后台聚合循环。
func (s *ChannelStatusService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *ChannelStatusService) currentRefreshInterval() time.Duration {
	cfg := s.settingService.GetChannelStatusConfigCached(context.Background())
	seconds := cfg.RefreshSeconds
	if seconds < MinChannelStatusRefreshSeconds {
		seconds = MinChannelStatusRefreshSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *ChannelStatusService) runOnceSafe() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ChannelStatus] aggregation panic recovered: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	snapshot, err := s.Aggregate(ctx)
	if err != nil {
		if !errors.Is(err, ErrChannelStatusDisabled) {
			log.Printf("[ChannelStatus] aggregation failed: %v", err)
		}
		return
	}
	s.StoreSnapshot(snapshot)
}

// Aggregate 聚合一次窗口数据并产出快照（不落缓存）。
func (s *ChannelStatusService) Aggregate(ctx context.Context) (*ChannelStatusSnapshot, error) {
	enabled, config, err := s.settingService.GetChannelStatusSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read channel status settings: %w", err)
	}
	if !enabled {
		return nil, ErrChannelStatusDisabled
	}

	window := time.Duration(config.WindowHours) * time.Hour
	now := timezone.Now()
	start := now.Add(-window)

	agg := newChannelStatusAgg(start, config.WindowHours)

	// 1) 成功请求（usage_logs）
	// 按「分组 × 模型 × 时间桶」直接以原始时间戳落桶（与失败侧、最近1小时口径
	// 完全同公式）。旧写法按 date_trunc('hour')+30m 落桶：窗口起点不对齐整点时
	// （start = now-24h 天然不对齐），边界小时会被整块错位到相邻桶，导致时间线
	// 最后一桶与「最近 1 小时」数字不一致。
	bucketSeconds := int64(config.WindowHours*3600) / channelStatusTimelineBuckets
	if bucketSeconds < 1 {
		bucketSeconds = 1
	}
	usageRows, err := s.db.QueryContext(ctx, `
		SELECT group_id,
		       coalesce(nullif(trim(requested_model), ''), model, '') AS model,
		       count(*) AS requests,
		       avg(duration_ms) AS avg_ms,
		       LEAST($3, GREATEST(0, floor(extract(epoch FROM (created_at - $1::timestamptz)) / $2)))::int AS bucket_idx
		FROM usage_logs
		WHERE created_at >= $1
		GROUP BY group_id, 2, 5
	`, start, bucketSeconds, channelStatusTimelineBuckets-1)
	if err != nil {
		return nil, fmt.Errorf("query usage aggregation: %w", err)
	}
	for usageRows.Next() {
		var groupID int64
		var model string
		var requests int64
		var avgMs *float64
		var bucketIdx int
		if err := usageRows.Scan(&groupID, &model, &requests, &avgMs, &bucketIdx); err != nil {
			_ = usageRows.Close()
			return nil, fmt.Errorf("scan usage row: %w", err)
		}
		var durationMs int64
		if avgMs != nil {
			durationMs = int64(*avgMs)
		}
		// 桶中点作时间戳（addSuccess 内部仅用它算桶下标，中点必落本桶）。
		agg.addSuccess(groupID, model, requests, durationMs, start.Add(time.Duration(bucketIdx)*time.Duration(bucketSeconds)*time.Second+time.Duration(bucketSeconds/2)*time.Second))
	}
	if err := usageRows.Err(); err != nil {
		_ = usageRows.Close()
		return nil, fmt.Errorf("iterate usage rows: %w", err)
	}
	_ = usageRows.Close()

	// 1.5) 最近 1 小时滚动口径的成功数与平均耗时（用户态默认视图）。
	lastHourCutoff := now.Add(-time.Hour)
	lastHourSuccess := map[int64]int64{}
	lastHourSumMs := map[int64]float64{}
	lastHourCntMs := map[int64]int64{}
	lastHourFailures := map[int64]int64{}
	lastHourRows, err := s.db.QueryContext(ctx, `
		SELECT group_id, count(*), avg(duration_ms)
		FROM usage_logs
		WHERE created_at >= $1 AND group_id IS NOT NULL
		GROUP BY group_id
	`, lastHourCutoff)
	if err != nil {
		return nil, fmt.Errorf("query last-hour usage: %w", err)
	}
	for lastHourRows.Next() {
		var groupID int64
		var requests int64
		var avgMs *float64
		if err := lastHourRows.Scan(&groupID, &requests, &avgMs); err != nil {
			_ = lastHourRows.Close()
			return nil, fmt.Errorf("scan last-hour usage row: %w", err)
		}
		lastHourSuccess[groupID] = requests
		if avgMs != nil {
			lastHourSumMs[groupID] = *avgMs * float64(requests)
			lastHourCntMs[groupID] = requests
		}
	}
	if err := lastHourRows.Err(); err != nil {
		_ = lastHourRows.Close()
		return nil, fmt.Errorf("iterate last-hour usage rows: %w", err)
	}
	_ = lastHourRows.Close()

	// 2) 失败请求（ops_error_logs，行级取回后在 Go 侧应用过滤规则）。
	// 消息列必须 error_message 优先：已恢复行的 upstream_error_message 是无
	// "Recovered" 标记的原始上游文案，若优先读它，Recovered 分类会全部失效
	// （被误判为 200 业务失败）。
	errorRows, err := s.db.QueryContext(ctx, `
		SELECT group_id,
		       coalesce(nullif(trim(requested_model), ''), model, '') AS model,
		       coalesce(upstream_status_code, 0) AS upstream_status_code,
		       coalesce(status_code, 0) AS status_code,
		       coalesce(nullif(error_message, ''), upstream_error_message, '') AS message,
		       created_at
		FROM ops_error_logs
		WHERE created_at >= $1 AND group_id IS NOT NULL
		ORDER BY created_at DESC
		LIMIT 50000
	`, start)
	if err != nil {
		return nil, fmt.Errorf("query error aggregation: %w", err)
	}
	for errorRows.Next() {
		var groupID int64
		var model string
		var upstreamStatus, clientStatus int
		var message string
		var createdAt time.Time
		if err := errorRows.Scan(&groupID, &model, &upstreamStatus, &clientStatus, &message, &createdAt); err != nil {
			_ = errorRows.Close()
			return nil, fmt.Errorf("scan error row: %w", err)
		}
		if MatchesChannelStatusFilterRules(config.FilterRules, upstreamStatus, clientStatus, message) {
			continue // 命中过滤规则：不计入失败
		}
		// 客户端误配模型名的请求(分组内无任何账号支持该模型):网关在路由阶段直接拒绝,
		// 不属于渠道故障,不计入统计。
		if strings.Contains(message, "is not supported by any configured account in this group") {
			continue
		}
		// status_code=200 的行细分：已恢复的中间失败（A 错 B 错 C 成功，客户端
		// 最终拿到 200）与误入的成功行不计入分组失败（用户视角：这次请求就是
		// 成功的，失败只在账号层面显示）；但池不可用/无可用账号/分组停用等
		// 业务失败虽是 200 也真实影响用户，保留计入。
		if clientStatus == http.StatusOK {
			if _, skip := classifyStatus200(message); skip || isRecoveredStatus200Row(message) {
				continue
			}
		}
		agg.addFailure(groupID, model, createdAt)
		if createdAt.After(lastHourCutoff) {
			lastHourFailures[groupID]++
		}
	}
	if err := errorRows.Err(); err != nil {
		_ = errorRows.Close()
		return nil, fmt.Errorf("iterate error rows: %w", err)
	}
	_ = errorRows.Close()

	// 2.5) 成功耗时分布（最快/最长/P95/P99）：一次扫描用 GROUPING SETS 同时算出
	// 全站(())、分组(group_id)、分组×模型(group_id, model)、分组×模型×时间桶
	// （时间线下钻视图用）四个粒度；percentile_cont 为插值精确值。
	// 与 avg_gen_ms 同源（duration_ms>0 的成功行），只统计成功请求。
	// 桶下标与成功侧/失败侧完全同公式：原始时间戳按桶宽取整并收敛到 [0,23]。
	latencyRows, err := s.db.QueryContext(ctx, `
		SELECT group_id, model, bucket_idx,
		       avg(ms)::float8,
		       min(ms)::float8, max(ms)::float8,
		       percentile_cont(0.95) WITHIN GROUP (ORDER BY ms),
		       percentile_cont(0.99) WITHIN GROUP (ORDER BY ms)
		FROM (
			SELECT group_id,
			       coalesce(nullif(trim(requested_model), ''), model, '') AS model,
			       LEAST($3, GREATEST(0, floor(extract(epoch FROM (created_at - $1::timestamptz)) / $2)))::int AS bucket_idx,
			       duration_ms AS ms
			FROM usage_logs
			WHERE created_at >= $1 AND duration_ms IS NOT NULL AND duration_ms > 0
		) t
		GROUP BY GROUPING SETS ((group_id, model, bucket_idx), (group_id, model), (group_id), ())
	`, start, bucketSeconds, channelStatusTimelineBuckets-1)
	if err != nil {
		return nil, fmt.Errorf("query latency stats: %w", err)
	}
	groupLatency := map[int64]channelLatencyStats{}
	modelLatency := map[int64]map[string]channelLatencyStats{}
	bucketLatency := map[int64]map[int]map[string]channelLatencyStats{}
	overallLatency := channelLatencyStats{}
	for latencyRows.Next() {
		var groupID, bucketIdx sql.NullInt64
		var model sql.NullString
		var avg, fastest, slowest, p95, p99 float64
		if err := latencyRows.Scan(&groupID, &model, &bucketIdx, &avg, &fastest, &slowest, &p95, &p99); err != nil {
			_ = latencyRows.Close()
			return nil, fmt.Errorf("scan latency row: %w", err)
		}
		stats := channelLatencyStats{
			AvgMs:     int64(math.Round(avg)),
			FastestMs: int64(math.Round(fastest)),
			SlowestMs: int64(math.Round(slowest)),
			P95Ms:     int64(math.Round(p95)),
			P99Ms:     int64(math.Round(p99)),
		}
		if !groupID.Valid {
			overallLatency = stats // () 分组：全站
			continue
		}
		id := groupID.Int64
		if !model.Valid {
			groupLatency[id] = stats // (group_id) 分组
			continue
		}
		if !bucketIdx.Valid {
			if modelLatency[id] == nil {
				modelLatency[id] = map[string]channelLatencyStats{}
			}
			modelLatency[id][model.String] = stats // (group_id, model) 分组
			continue
		}
		if bucketLatency[id] == nil {
			bucketLatency[id] = map[int]map[string]channelLatencyStats{}
		}
		if bucketLatency[id][int(bucketIdx.Int64)] == nil {
			bucketLatency[id][int(bucketIdx.Int64)] = map[string]channelLatencyStats{}
		}
		bucketLatency[id][int(bucketIdx.Int64)][model.String] = stats
	}
	if err := latencyRows.Err(); err != nil {
		_ = latencyRows.Close()
		return nil, fmt.Errorf("iterate latency rows: %w", err)
	}
	_ = latencyRows.Close()

	// 3) 分组元信息（名称/平台/是否专属）
	groupMeta := map[int64]channelStatusGroupMeta{}
	metaRows, err := s.db.QueryContext(ctx, `SELECT id, name, coalesce(platform, ''), coalesce(is_exclusive, false) FROM groups`)
	if err != nil {
		return nil, fmt.Errorf("query groups: %w", err)
	}
	for metaRows.Next() {
		var id int64
		var name, platform string
		var exclusive bool
		if err := metaRows.Scan(&id, &name, &platform, &exclusive); err != nil {
			_ = metaRows.Close()
			return nil, fmt.Errorf("scan group row: %w", err)
		}
		groupMeta[id] = channelStatusGroupMeta{Name: name, Platform: platform, Exclusive: exclusive}
	}
	if err := metaRows.Err(); err != nil {
		_ = metaRows.Close()
		return nil, fmt.Errorf("iterate groups: %w", err)
	}
	_ = metaRows.Close()

	// 4) 组装快照：先收集窗口内有记录的分组 ID（升序），再按公开性/展示白名单筛选
	ids := make([]int64, 0, len(agg.successRequests))
	for id := range agg.successRequests {
		ids = append(ids, id)
	}
	for id := range agg.failures {
		if _, ok := agg.successRequests[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	ids = channelStatusDisplayIDs(ids, config.DisplayGroupIDs, groupMeta)

	groups := make([]GroupStatus, 0, len(ids))
	overallDown, overallDegraded := false, false
	for _, id := range ids {
		name := fmt.Sprintf("group-%d", id)
		platform := ""
		if meta, ok := groupMeta[id]; ok {
			name = strings.TrimSpace(meta.Name)
			platform = meta.Platform
		}
		gr := GroupStatus{ID: id, Name: name, Platform: platform}

		modelSet := map[string]struct{}{}
		for m := range agg.successRequests[id] {
			modelSet[m] = struct{}{}
		}
		for m := range agg.failures[id] {
			modelSet[m] = struct{}{}
		}
		var groupSumMs float64
		var groupCntMs int64
		for m := range modelSet {
			requests := agg.successRequests[id][m]
			failures := agg.failures[id][m]
			total := requests + failures
			var avgMs int64
			if agg.successCntMs[id] != nil && agg.successCntMs[id][m] > 0 {
				avgMs = int64(math.Round(agg.successSumMs[id][m] / float64(agg.successCntMs[id][m])))
				groupSumMs += agg.successSumMs[id][m]
				groupCntMs += agg.successCntMs[id][m]
			}
			ml := modelLatency[id][m]
			gr.Models = append(gr.Models, ChannelStatusModelStatus{
				Model: m, Requests: requests, Failures: failures,
				SuccessRate: ratePercent(requests, total), AvgGenMs: avgMs,
				FastestMs: ml.FastestMs, SlowestMs: ml.SlowestMs, P95Ms: ml.P95Ms, P99Ms: ml.P99Ms,
			})
			gr.Requests += requests
			gr.Failures += failures
		}
		sort.Slice(gr.Models, func(i, j int) bool { return gr.Models[i].Requests > gr.Models[j].Requests })
		gr.SuccessRate = ratePercent(gr.Requests, gr.Requests+gr.Failures)
		if groupCntMs > 0 {
			gr.AvgGenMs = int64(math.Round(groupSumMs / float64(groupCntMs)))
		}
		if gl, ok := groupLatency[id]; ok {
			gr.FastestMs, gr.SlowestMs, gr.P95Ms, gr.P99Ms = gl.FastestMs, gl.SlowestMs, gl.P95Ms, gl.P99Ms
		}
		lhSuccess := lastHourSuccess[id]
		lhFailures := lastHourFailures[id]
		if lhSuccess+lhFailures > 0 {
			var lhAvg int64
			if lastHourCntMs[id] > 0 {
				lhAvg = int64(math.Round(lastHourSumMs[id] / float64(lastHourCntMs[id])))
			}
			gr.LastHour = &ChannelStatusLastHour{
				Requests:    lhSuccess,
				Failures:    lhFailures,
				SuccessRate: ratePercent(lhSuccess, lhSuccess+lhFailures),
				AvgGenMs:    lhAvg,
			}
		}
		gr.Status = classifyChannelStatus(gr.SuccessRate, gr.Requests+gr.Failures > 0, config.Thresholds)

		// 时间线（桶数固定 24，桶宽 = 窗口/24，与耗时分布查询同参数）
		requestsB := agg.bucketRequests[id]
		failuresB := agg.bucketFailures[id]
		gr.Timeline = make([]ChannelStatusBucket, 0, channelStatusTimelineBuckets)
		for i := 0; i < channelStatusTimelineBuckets; i++ {
			bStart := start.Add(time.Duration(i) * time.Duration(bucketSeconds) * time.Second)
			requests := requestsB[i]
			failures := failuresB[i]
			total := requests + failures
			gr.Timeline = append(gr.Timeline, ChannelStatusBucket{
				BucketStartMs: bStart.UnixMilli(),
				Requests:      requests,
				Failures:      failures,
				SuccessRate:   ratePercent(requests, total),
				Status:        classifyChannelStatus(ratePercent(requests, total), total > 0, config.Thresholds),
				Models:        agg.bucketModels(id, i, bucketLatency[id]),
			})
		}

		switch gr.Status {
		case "down":
			overallDown = true
		case "degraded":
			overallDegraded = true
		}
		groups = append(groups, gr)
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].Requests > groups[j].Requests })

	overall := "unknown"
	if len(groups) > 0 {
		switch {
		case overallDown:
			overall = "down"
		case overallDegraded:
			overall = "degraded"
		default:
			overall = "operational"
		}
	}

	return &ChannelStatusSnapshot{
		GeneratedAtMs: now.UnixMilli(),
		WindowHours:   config.WindowHours,
		RefreshSec:    config.RefreshSeconds,
		Overall:       overall,
		Thresholds:    config.Thresholds,
		AvgGenMs:      overallLatency.AvgMs,
		FastestMs:     overallLatency.FastestMs,
		SlowestMs:     overallLatency.SlowestMs,
		P95Ms:         overallLatency.P95Ms,
		P99Ms:         overallLatency.P99Ms,
		Groups:        groups,
	}, nil
}

// PublicView 返回脱敏后的快照副本：耗时分布（最快/最长/P95/P99）属于
// 管理端详细指标，匿名/普通用户不可见——公开接口只保留成功率、平均耗时
// 与最近 1 小时口径。平均耗时对用户可见（默认展示最近 1 小时口径）。
func (s *ChannelStatusSnapshot) PublicView() *ChannelStatusSnapshot {
	if s == nil {
		return nil
	}
	clone := *s
	clone.FastestMs, clone.SlowestMs, clone.P95Ms, clone.P99Ms = 0, 0, 0, 0
	clone.Groups = make([]GroupStatus, len(s.Groups))
	for i, g := range s.Groups {
		gc := g
		gc.FastestMs, gc.SlowestMs, gc.P95Ms, gc.P99Ms = 0, 0, 0, 0
		gc.Models = make([]ChannelStatusModelStatus, len(g.Models))
		for j, m := range g.Models {
			mc := m
			mc.FastestMs, mc.SlowestMs, mc.P95Ms, mc.P99Ms = 0, 0, 0, 0
			gc.Models[j] = mc
		}
		gc.Timeline = make([]ChannelStatusBucket, len(g.Timeline))
		for k, b := range g.Timeline {
			bc := b
			bc.Models = make([]ChannelStatusModelStatus, len(b.Models))
			for j, m := range b.Models {
				mc := m
				mc.FastestMs, mc.SlowestMs, mc.P95Ms, mc.P99Ms = 0, 0, 0, 0
				bc.Models[j] = mc
			}
			gc.Timeline[k] = bc
		}
		clone.Groups[i] = gc
	}
	return &clone
}

// StoreSnapshot 把快照写入内存 + Redis。
func (s *ChannelStatusService) StoreSnapshot(snapshot *ChannelStatusSnapshot) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("[ChannelStatus] marshal snapshot failed: %v", err)
		return
	}
	s.mu.Lock()
	s.snapshot = snapshot
	s.mu.Unlock()

	if s.redisClient != nil {
		ttl := time.Duration(s.currentRefreshInterval())*3 + 30*time.Second
		if err := s.redisClient.Set(context.Background(), channelStatusSnapshotRedisKey, payload, ttl).Err(); err != nil {
			log.Printf("[ChannelStatus] write redis snapshot failed: %v", err)
		}
	}
}

// GetCachedSnapshot 返回缓存快照：内存优先，回退 Redis。
func (s *ChannelStatusService) GetCachedSnapshot(ctx context.Context) (*ChannelStatusSnapshot, error) {
	s.mu.RLock()
	snapshot := s.snapshot
	s.mu.RUnlock()
	if snapshot != nil {
		return snapshot, nil
	}
	if s.redisClient != nil {
		payload, err := s.redisClient.Get(ctx, channelStatusSnapshotRedisKey).Result()
		if err == nil {
			var cached ChannelStatusSnapshot
			if err := json.Unmarshal([]byte(payload), &cached); err == nil {
				return &cached, nil
			}
		}
	}
	return nil, ErrChannelStatusNoSnapshot
}

// ---- 错误 ----

var (
	ErrChannelStatusDisabled   = errors.New("channel status is disabled")
	ErrChannelStatusNoSnapshot = errors.New("channel status snapshot not ready")
)

// ---- 管理端：分组内账号成功率/请求数（图片计费控制台） ----

// AccountSuccessStat 单账号在窗口内的成功/失败统计。
type AccountSuccessStat struct {
	AccountID int64 `json:"account_id"`
	Requests  int64 `json:"requests"`
	Failures  int64 `json:"failures"`
	// SwitchedToRequests 是"经账号切换后最终落在本账号成功"的请求数
	//（usage_logs.account_attempts 链长 >1），衡量链路转移到达量。
	SwitchedToRequests int64 `json:"switched_to_requests,omitempty"`
	// ErrorsByCode 是窗口/区间内该账号失败记录的状态码分类
	//（status_code 计数 + 截断后的代表性错误消息），管理员排障用。
	ErrorsByCode []AccountErrorClass `json:"errors_by_code,omitempty"`
	SuccessRate  float64             `json:"success_rate"` // 百分比 0-100；无请求时 -1 表示 unknown
	AvgGenMs     int64               `json:"avg_gen_ms"`
	FastestMs    int64               `json:"fastest_ms"` // 该账号成功请求的耗时分布
	SlowestMs    int64               `json:"slowest_ms"`
	P95Ms        int64               `json:"p95_ms"`
	P99Ms        int64               `json:"p99_ms"`
	Hourly       []AccountHourlyStat `json:"hourly"` // 仅 24h 窗口时返回：逐小时明细
}

// AccountHourlyStat 单账号单小时的成功/失败统计（状态分级口径同状态页阈值）。
type AccountHourlyStat struct {
	HourStartMs int64   `json:"hour_start_ms"`
	Requests    int64   `json:"requests"`
	Failures    int64   `json:"failures"`
	SuccessRate float64 `json:"success_rate"` // -1 表示该小时无请求
	AvgGenMs    int64   `json:"avg_gen_ms"`
	Status      string  `json:"status"` // operational | degraded | down | unknown
}

// AccountSuccessStats 统计指定分组在窗口内每个账号的成功率、请求数与平均生图耗时。
// 失败口径与渠道状态页一致：应用后台过滤规则、排除「模型不被分组内任何账号支持」
// 的误配请求。渠道状态功能未开启时仍可用（过滤规则读不到则视为不过滤）。
// 返回值第二个参数为实际生效的窗口小时数（调用方超范围时被收敛）。
// AccountErrorClass 是账号在统计区间内某一类失败（按状态码归类）的计数与样例消息。
type AccountErrorClass struct {
	Status        string `json:"status"`
	Count         int64  `json:"count"`
	SampleMessage string `json:"sample_message,omitempty"`
}

func (s *ChannelStatusService) AccountSuccessStats(ctx context.Context, groupID int64, windowHours int) ([]AccountSuccessStat, int, error) {
	return s.AccountSuccessStatsRange(ctx, groupID, windowHours, time.Time{}, time.Time{})
}

// AccountSuccessStatsRange 支持任意 [start,end] 区间统计（管理员账号明细下钻用）。
// start/end 为零值时回退 windowHours 回溯窗口；区间上限同样 30 天。
func (s *ChannelStatusService) AccountSuccessStatsRange(ctx context.Context, groupID int64, windowHours int, startAt, endAt time.Time) ([]AccountSuccessStat, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, errors.New("channel status service is not ready")
	}
	if groupID <= 0 {
		return nil, 0, errors.New("group_id is required")
	}
	if windowHours <= 0 {
		windowHours = 24
	}
	if windowHours > 24*30 {
		windowHours = 24 * 30
	}
	now := timezone.Now()
	start := now.Add(-time.Duration(windowHours) * time.Hour)
	if !startAt.IsZero() {
		start = startAt
	}
	end := now
	if !endAt.IsZero() {
		end = endAt
	}
	// 区间模式下逐小时明细退化为"按小时重建"：仍用 24 桶锚定区间起点，
	// 超过 24h 的区间不输出 hourly（前端只显示汇总行）。
	windowHours = int(end.Sub(start).Hours() + 0.5)

	var rules []ChannelStatusFilterRule
	var thresholds ChannelStatusThresholds
	if s.settingService != nil {
		if _, cfg, err := s.settingService.GetChannelStatusSettings(ctx); err == nil {
			rules = cfg.FilterRules
			thresholds = cfg.Thresholds
		}
	}

	requestsByAccount := map[int64]int64{}
	sumMsByAccount := map[int64]float64{}
	cntMsByAccount := map[int64]int64{}
	failuresByAccount := map[int64]int64{}
	switchedByAccount := map[int64]int64{}
	errorsByCodeAccount := map[int64]map[string]int64{}
	errorSampleAccount := map[int64]map[string]string{}
	errorClassesByAccount := map[int64][]AccountErrorClass{}
	// 逐小时明细（仅 24h 窗口）：account_id → hour index → 统计
	const hourlyBuckets = 24
	hourlyRequests := map[int64][]int64{}
	hourlyFailures := map[int64][]int64{}
	hourlySumMs := map[int64][]float64{}
	hourlyCntMs := map[int64][]int64{}
	ensureHourly := func(accountID int64) {
		if hourlyRequests[accountID] == nil {
			hourlyRequests[accountID] = make([]int64, hourlyBuckets)
			hourlyFailures[accountID] = make([]int64, hourlyBuckets)
			hourlySumMs[accountID] = make([]float64, hourlyBuckets)
			hourlyCntMs[accountID] = make([]int64, hourlyBuckets)
		}
	}
	hourIndex := func(at time.Time) int {
		idx := int(at.Sub(start).Hours())
		if idx < 0 {
			return 0
		}
		if idx >= hourlyBuckets {
			return hourlyBuckets - 1
		}
		return idx
	}

	usageRows, err := s.db.QueryContext(ctx, `
		SELECT account_id,
		       count(*) AS requests,
		       avg(duration_ms) AS avg_ms,
		       count(*) FILTER (WHERE jsonb_array_length(account_attempts) > 1) AS switched,
		       date_trunc('hour', created_at) AS bucket_hour
		FROM usage_logs
		WHERE group_id = $1 AND created_at >= $2 AND created_at <= $3 AND account_id IS NOT NULL
		GROUP BY account_id, date_trunc('hour', created_at)
	`, groupID, start, end)
	if err != nil {
		return nil, 0, fmt.Errorf("query account usage: %w", err)
	}
	for usageRows.Next() {
		var accountID int64
		var requests int64
		var avgMs *float64
		var switched int64
		var bucketHour time.Time
		if err := usageRows.Scan(&accountID, &requests, &avgMs, &switched, &bucketHour); err != nil {
			_ = usageRows.Close()
			return nil, 0, fmt.Errorf("scan account usage row: %w", err)
		}
		requestsByAccount[accountID] += requests
		switchedByAccount[accountID] += switched
		if avgMs != nil {
			sumMsByAccount[accountID] += *avgMs * float64(requests)
			cntMsByAccount[accountID] += requests
		}
		ensureHourly(accountID)
		idx := hourIndex(bucketHour.Add(30 * time.Minute))
		hourlyRequests[accountID][idx] += requests
		if avgMs != nil {
			hourlySumMs[accountID][idx] += *avgMs * float64(requests)
			hourlyCntMs[accountID][idx] += requests
		}
	}
	if err := usageRows.Err(); err != nil {
		_ = usageRows.Close()
		return nil, 0, fmt.Errorf("iterate account usage rows: %w", err)
	}
	_ = usageRows.Close()

	errorRows, err := s.db.QueryContext(ctx, `
		SELECT account_id,
		       coalesce(upstream_status_code, 0) AS upstream_status_code,
		       coalesce(status_code, 0) AS status_code,
		       coalesce(nullif(error_message, ''), upstream_error_message, '') AS message,
		       created_at
		FROM ops_error_logs
		WHERE group_id = $1 AND created_at >= $2 AND created_at <= $3 AND account_id IS NOT NULL
		ORDER BY created_at DESC
		LIMIT 50000
	`, groupID, start, end)
	if err != nil {
		return nil, 0, fmt.Errorf("query account errors: %w", err)
	}
	for errorRows.Next() {
		var accountID int64
		var upstreamStatus, clientStatus int
		var message string
		var createdAt time.Time
		if err := errorRows.Scan(&accountID, &upstreamStatus, &clientStatus, &message, &createdAt); err != nil {
			_ = errorRows.Close()
			return nil, 0, fmt.Errorf("scan account error row: %w", err)
		}
		if MatchesChannelStatusFilterRules(rules, upstreamStatus, clientStatus, message) {
			continue // 与状态页同口径：命中过滤规则的失败不计入
		}
		if strings.Contains(message, "is not supported by any configured account in this group") {
			continue // 客户端误配模型名的路由拒绝，不属于渠道故障
		}
		// 错误分类码：200 行细分（已恢复 ↻N / ↻net、内嵌真实码、200）；
		// 误入错误表的成功行在账号层面也不计失败，直接跳过。
		classStatus := fmt.Sprintf("%d", clientStatus)
		if clientStatus == http.StatusOK {
			var skipRow bool
			classStatus, skipRow = classifyStatus200(message)
			if skipRow {
				continue
			}
		}
		failuresByAccount[accountID]++
		if windowHours == hourlyBuckets {
			ensureHourly(accountID)
			hourlyFailures[accountID][hourIndex(createdAt)]++
		}
		codeKey := classStatus
		if errorsByCodeAccount[accountID] == nil {
			errorsByCodeAccount[accountID] = map[string]int64{}
			errorSampleAccount[accountID] = map[string]string{}
		}
		errorsByCodeAccount[accountID][codeKey]++
		sampleMsg := strings.TrimSpace(message)
		if len(sampleMsg) > 100 {
			sampleMsg = sampleMsg[:100] + "…"
		}
		if sampleMsg != "" {
			// 每类只保留第一条（最新，因 ORDER BY created_at DESC）作为样例。
			// key 与 errorsByCodeAccount 一致（200 行是分类码如 "↻504"，非原始状态码）。
			if _, exists := errorSampleAccount[accountID][codeKey]; !exists {
				errorSampleAccount[accountID][codeKey] = sampleMsg
			}
		}
	}
	if err := errorRows.Err(); err != nil {
		_ = errorRows.Close()
		return nil, 0, fmt.Errorf("iterate account error rows: %w", err)
	}
	_ = errorRows.Close()

	// 账号级成功耗时分布（最快/最长/P95/P99），与 avg_gen_ms 同源同窗口。
	latencyRows, err := s.db.QueryContext(ctx, `
		SELECT account_id,
		       min(duration_ms)::float8, max(duration_ms)::float8,
		       percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms),
		       percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms)
		FROM usage_logs
		WHERE group_id = $1 AND created_at >= $2 AND created_at <= $3
		  AND account_id IS NOT NULL AND duration_ms IS NOT NULL AND duration_ms > 0
		GROUP BY account_id
	`, groupID, start, end)
	if err != nil {
		return nil, 0, fmt.Errorf("query account latency stats: %w", err)
	}
	latencyByAccount := map[int64]channelLatencyStats{}
	for latencyRows.Next() {
		var accountID int64
		var fastest, slowest, p95, p99 float64
		if err := latencyRows.Scan(&accountID, &fastest, &slowest, &p95, &p99); err != nil {
			_ = latencyRows.Close()
			return nil, 0, fmt.Errorf("scan account latency row: %w", err)
		}
		latencyByAccount[accountID] = channelLatencyStats{
			FastestMs: int64(math.Round(fastest)),
			SlowestMs: int64(math.Round(slowest)),
			P95Ms:     int64(math.Round(p95)),
			P99Ms:     int64(math.Round(p99)),
		}
	}
	if err := latencyRows.Err(); err != nil {
		_ = latencyRows.Close()
		return nil, 0, fmt.Errorf("iterate account latency rows: %w", err)
	}
	_ = latencyRows.Close()

	accounts := make([]AccountSuccessStat, 0, len(requestsByAccount)+len(failuresByAccount))
	seen := map[int64]struct{}{}
	for accountID := range requestsByAccount {
		seen[accountID] = struct{}{}
	}
	for accountID := range failuresByAccount {
		seen[accountID] = struct{}{}
	}
	for accountID := range seen {
		requests := requestsByAccount[accountID]
		failures := failuresByAccount[accountID]
		var avgMs int64
		if cntMsByAccount[accountID] > 0 {
			avgMs = int64(math.Round(sumMsByAccount[accountID] / float64(cntMsByAccount[accountID])))
		}
		if codes := errorsByCodeAccount[accountID]; len(codes) > 0 {
			classes := make([]AccountErrorClass, 0, len(codes))
			for code, cnt := range codes {
				classes = append(classes, AccountErrorClass{
					Status:        code,
					Count:         cnt,
					SampleMessage: errorSampleAccount[accountID][code],
				})
			}
			sort.Slice(classes, func(i, j int) bool { return classes[i].Count > classes[j].Count })
			if len(classes) > 6 {
				classes = classes[:6]
			}
			errorClassesByAccount[accountID] = classes
		}
		stat := AccountSuccessStat{
			AccountID:          accountID,
			Requests:           requests,
			Failures:           failures,
			SwitchedToRequests: switchedByAccount[accountID],
			SuccessRate:        ratePercent(requests, requests+failures),
			AvgGenMs:           avgMs,
			FastestMs:          latencyByAccount[accountID].FastestMs,
			SlowestMs:          latencyByAccount[accountID].SlowestMs,
			P95Ms:              latencyByAccount[accountID].P95Ms,
			P99Ms:              latencyByAccount[accountID].P99Ms,
			ErrorsByCode:       errorClassesByAccount[accountID],
		}
		// 仅 24h 窗口输出逐小时明细；桶时间锚定在窗口起点，与账号表汇总同源。
		if windowHours == hourlyBuckets && hourlyRequests[accountID] != nil {
			for i := 0; i < hourlyBuckets; i++ {
				hReq := hourlyRequests[accountID][i]
				hFail := hourlyFailures[accountID][i]
				hTotal := hReq + hFail
				var hAvgMs int64
				if hourlyCntMs[accountID][i] > 0 {
					hAvgMs = int64(math.Round(hourlySumMs[accountID][i] / float64(hourlyCntMs[accountID][i])))
				}
				stat.Hourly = append(stat.Hourly, AccountHourlyStat{
					HourStartMs: start.Add(time.Duration(i) * time.Hour).UnixMilli(),
					Requests:    hReq,
					Failures:    hFail,
					SuccessRate: ratePercent(hReq, hTotal),
					AvgGenMs:    hAvgMs,
					Status:      classifyChannelStatus(ratePercent(hReq, hTotal), hTotal > 0, thresholds),
				})
			}
		}
		accounts = append(accounts, stat)
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Requests != accounts[j].Requests {
			return accounts[i].Requests > accounts[j].Requests
		}
		return accounts[i].AccountID < accounts[j].AccountID
	})
	return accounts, windowHours, nil
}
