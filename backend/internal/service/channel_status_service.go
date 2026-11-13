package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
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

// ---- 对外数据结构（JSON 即 API 响应契约，字段只含可公开展示信息） ----

type ChannelStatusModelStatus struct {
	Model       string  `json:"model"`
	Requests    int64   `json:"requests"`
	Failures    int64   `json:"failures"`
	SuccessRate float64 `json:"success_rate"` // 百分比 0-100；无请求时 -1 表示 unknown
	AvgGenMs    int64   `json:"avg_gen_ms"`   // 成功请求平均耗时；0 表示无样本
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

type GroupStatus struct {
	ID          int64                      `json:"id"`
	Name        string                     `json:"name"`
	Platform    string                     `json:"platform"`
	Status      string                     `json:"status"` // operational | degraded | down | unknown
	Requests    int64                      `json:"requests"`
	Failures    int64                      `json:"failures"`
	SuccessRate float64                    `json:"success_rate"`
	AvgGenMs    int64                      `json:"avg_gen_ms"`
	Models      []ChannelStatusModelStatus `json:"models"`
	Timeline    []ChannelStatusBucket      `json:"timeline"`
}

type ChannelStatusSnapshot struct {
	GeneratedAtMs int64         `json:"generated_at_ms"`
	WindowHours   int           `json:"window_hours"`
	RefreshSec    int           `json:"refresh_seconds"`
	Overall       string        `json:"overall"` // operational | degraded | down | unknown
	Groups        []GroupStatus `json:"groups"`
}

const channelStatusTimelineBuckets = 24

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
func (a *channelStatusAgg) bucketModels(accountID int64, idx int) []ChannelStatusModelStatus {
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
	for m := range modelSet {
		reqN := requests[m]
		failN := failures[m]
		total := reqN + failN
		var avgMs int64
		if cntMs != nil && cntMs[m] > 0 {
			avgMs = int64(math.Round(sumMs[m] / float64(cntMs[m])))
		}
		models = append(models, ChannelStatusModelStatus{
			Model:       m,
			Requests:    reqN,
			Failures:    failN,
			SuccessRate: ratePercent(reqN, total),
			AvgGenMs:    avgMs,
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
	// 按「分组 × 模型 × 小时桶」分组：时间线的每个桶必须拿到自己小时内的成功数。
	// 旧写法只按 (group_id, model) 聚合并取 max(created_at) 当桶时间戳，
	// 会把整个窗口的成功请求全部堆进最近一小时的桶，历史小时桶只剩失败 → 成功率 0。
	usageRows, err := s.db.QueryContext(ctx, `
		SELECT group_id,
		       coalesce(nullif(trim(requested_model), ''), model, '') AS model,
		       count(*) AS requests,
		       avg(duration_ms) AS avg_ms,
		       date_trunc('hour', created_at) AS bucket_hour
		FROM usage_logs
		WHERE created_at >= $1
		GROUP BY group_id, 2, date_trunc('hour', created_at)
	`, start)
	if err != nil {
		return nil, fmt.Errorf("query usage aggregation: %w", err)
	}
	for usageRows.Next() {
		var groupID int64
		var model string
		var requests int64
		var avgMs *float64
		var bucketHour time.Time
		if err := usageRows.Scan(&groupID, &model, &requests, &avgMs, &bucketHour); err != nil {
			_ = usageRows.Close()
			return nil, fmt.Errorf("scan usage row: %w", err)
		}
		var durationMs int64
		if avgMs != nil {
			durationMs = int64(*avgMs)
		}
		// 取小时桶中点作时间戳：整小时的请求全部落进覆盖该小时中点的同一个桶，
		// 与失败侧行级时间戳的落桶口径一致，不会在桶边界上劈成两半。
		agg.addSuccess(groupID, model, requests, durationMs, bucketHour.Add(30*time.Minute))
	}
	if err := usageRows.Err(); err != nil {
		_ = usageRows.Close()
		return nil, fmt.Errorf("iterate usage rows: %w", err)
	}
	_ = usageRows.Close()

	// 2) 失败请求（ops_error_logs，行级取回后在 Go 侧应用过滤规则）
	errorRows, err := s.db.QueryContext(ctx, `
		SELECT group_id,
		       coalesce(nullif(trim(requested_model), ''), model, '') AS model,
		       coalesce(upstream_status_code, 0) AS upstream_status_code,
		       coalesce(status_code, 0) AS status_code,
		       coalesce(upstream_error_message, error_message, '') AS message,
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
		agg.addFailure(groupID, model, createdAt)
	}
	if err := errorRows.Err(); err != nil {
		_ = errorRows.Close()
		return nil, fmt.Errorf("iterate error rows: %w", err)
	}
	_ = errorRows.Close()

	// 3) 分组元信息（名称/平台/是否专属）
	groupMeta := map[int64]struct {
		Name      string
		Platform  string
		Exclusive bool
	}{}
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
		groupMeta[id] = struct {
			Name      string
			Platform  string
			Exclusive bool
		}{Name: name, Platform: platform, Exclusive: exclusive}
	}
	if err := metaRows.Err(); err != nil {
		_ = metaRows.Close()
		return nil, fmt.Errorf("iterate groups: %w", err)
	}
	_ = metaRows.Close()

	// 4) 组装快照
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

	// 只统计公开（非专属）分组：专属分组不进快照；已删除的分组（无元信息）同样跳过
	publicIDs := make([]int64, 0, len(ids))
	for _, id := range ids {
		meta, ok := groupMeta[id]
		if !ok || meta.Exclusive {
			continue
		}
		publicIDs = append(publicIDs, id)
	}
	ids = publicIDs

	// 展示白名单：配置了 display_group_ids 时只输出这些分组
	if len(config.DisplayGroupIDs) > 0 {
		allowed := make(map[int64]struct{}, len(config.DisplayGroupIDs))
		for _, id := range config.DisplayGroupIDs {
			allowed[id] = struct{}{}
		}
		filtered := ids[:0]
		for _, id := range ids {
			if _, ok := allowed[id]; ok {
				filtered = append(filtered, id)
			}
		}
		ids = filtered
	}

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
			gr.Models = append(gr.Models, ChannelStatusModelStatus{
				Model: m, Requests: requests, Failures: failures,
				SuccessRate: ratePercent(requests, total), AvgGenMs: avgMs,
			})
			gr.Requests += requests
			gr.Failures += failures
		}
		sort.Slice(gr.Models, func(i, j int) bool { return gr.Models[i].Requests > gr.Models[j].Requests })
		gr.SuccessRate = ratePercent(gr.Requests, gr.Requests+gr.Failures)
		if groupCntMs > 0 {
			gr.AvgGenMs = int64(math.Round(groupSumMs / float64(groupCntMs)))
		}
		gr.Status = classifyChannelStatus(gr.SuccessRate, gr.Requests+gr.Failures > 0, config.Thresholds)

		// 时间线（桶数固定 24，桶宽 = 窗口/24）
		bucketSeconds := int64(config.WindowHours*3600) / channelStatusTimelineBuckets
		if bucketSeconds < 1 {
			bucketSeconds = 1
		}
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
				Models:        agg.bucketModels(id, i),
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
		Groups:        groups,
	}, nil
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
	AccountID   int64               `json:"account_id"`
	Requests    int64               `json:"requests"`
	Failures    int64               `json:"failures"`
	SuccessRate float64             `json:"success_rate"` // 百分比 0-100；无请求时 -1 表示 unknown
	AvgGenMs    int64               `json:"avg_gen_ms"`
	Hourly      []AccountHourlyStat `json:"hourly"` // 仅 24h 窗口时返回：逐小时明细
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
func (s *ChannelStatusService) AccountSuccessStats(ctx context.Context, groupID int64, windowHours int) ([]AccountSuccessStat, int, error) {
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
	start := timezone.Now().Add(-time.Duration(windowHours) * time.Hour)

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
		       date_trunc('hour', created_at) AS bucket_hour
		FROM usage_logs
		WHERE group_id = $1 AND created_at >= $2 AND account_id IS NOT NULL
		GROUP BY account_id, date_trunc('hour', created_at)
	`, groupID, start)
	if err != nil {
		return nil, 0, fmt.Errorf("query account usage: %w", err)
	}
	for usageRows.Next() {
		var accountID int64
		var requests int64
		var avgMs *float64
		var bucketHour time.Time
		if err := usageRows.Scan(&accountID, &requests, &avgMs, &bucketHour); err != nil {
			_ = usageRows.Close()
			return nil, 0, fmt.Errorf("scan account usage row: %w", err)
		}
		requestsByAccount[accountID] += requests
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
		       coalesce(upstream_error_message, error_message, '') AS message,
		       created_at
		FROM ops_error_logs
		WHERE group_id = $1 AND created_at >= $2 AND account_id IS NOT NULL
		ORDER BY created_at DESC
		LIMIT 50000
	`, groupID, start)
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
		failuresByAccount[accountID]++
		if windowHours == hourlyBuckets {
			ensureHourly(accountID)
			hourlyFailures[accountID][hourIndex(createdAt)]++
		}
	}
	if err := errorRows.Err(); err != nil {
		_ = errorRows.Close()
		return nil, 0, fmt.Errorf("iterate account error rows: %w", err)
	}
	_ = errorRows.Close()

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
		stat := AccountSuccessStat{
			AccountID:   accountID,
			Requests:    requests,
			Failures:    failures,
			SuccessRate: ratePercent(requests, requests+failures),
			AvgGenMs:    avgMs,
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
