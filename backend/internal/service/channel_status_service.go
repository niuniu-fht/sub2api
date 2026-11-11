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
}

func newChannelStatusAgg(start time.Time, windowHours int) *channelStatusAgg {
	return &channelStatusAgg{
		successRequests: map[int64]map[string]int64{},
		successSumMs:    map[int64]map[string]float64{},
		successCntMs:    map[int64]map[string]int64{},
		failures:        map[int64]map[string]int64{},
		bucketStart:     start,
		bucketSeconds:   float64(windowHours*3600) / float64(channelStatusTimelineBuckets),
		bucketRequests:  map[int64][]int64{},
		bucketFailures:  map[int64][]int64{},
	}
}

func (a *channelStatusAgg) ensureBucket(accountID int64) {
	if a.bucketRequests[accountID] == nil {
		a.bucketRequests[accountID] = make([]int64, channelStatusTimelineBuckets)
	}
	if a.bucketFailures[accountID] == nil {
		a.bucketFailures[accountID] = make([]int64, channelStatusTimelineBuckets)
	}
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

func (a *channelStatusAgg) addSuccess(accountID int64, model string, durationMs int64, at time.Time) {
	if a.successRequests[accountID] == nil {
		a.successRequests[accountID] = map[string]int64{}
		a.successSumMs[accountID] = map[string]float64{}
		a.successCntMs[accountID] = map[string]int64{}
	}
	a.successRequests[accountID][model]++
	if durationMs > 0 {
		a.successSumMs[accountID][model] += float64(durationMs)
		a.successCntMs[accountID][model]++
	}
	a.ensureBucket(accountID)
	a.bucketRequests[accountID][a.bucketIndexAt(at)]++
}

func (a *channelStatusAgg) addFailure(accountID int64, model string, at time.Time) {
	if a.failures[accountID] == nil {
		a.failures[accountID] = map[string]int64{}
	}
	a.failures[accountID][model]++
	a.ensureBucket(accountID)
	a.bucketFailures[accountID][a.bucketIndexAt(at)]++
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
	usageRows, err := s.db.QueryContext(ctx, `
		SELECT group_id,
		       coalesce(nullif(trim(requested_model), ''), model, '') AS model,
		       count(*) AS requests,
		       avg(duration_ms) AS avg_ms,
		       max(created_at) AS last_at
		FROM usage_logs
		WHERE created_at >= $1
		GROUP BY group_id, 2
	`, start)
	if err != nil {
		return nil, fmt.Errorf("query usage aggregation: %w", err)
	}
	for usageRows.Next() {
		var groupID int64
		var model string
		var requests int64
		var avgMs *float64
		var lastAt time.Time
		if err := usageRows.Scan(&groupID, &model, &requests, &avgMs, &lastAt); err != nil {
			_ = usageRows.Close()
			return nil, fmt.Errorf("scan usage row: %w", err)
		}
		var durationMs int64
		if avgMs != nil {
			durationMs = int64(*avgMs)
		}
		agg.addSuccess(groupID, model, durationMs, lastAt)
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
