package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// 渠道状态统计（channel status）settings keys。
// 数据完全来自真实生成记录（usage_logs / ops_error_logs）的被动聚合，无主动探测。
const (
	SettingKeyChannelStatusEnabled = "channel_status_enabled"
	SettingKeyChannelStatusConfig  = "channel_status_config"

	channelStatusSettingsCacheTTL = 30 * time.Second
)

// ChannelStatusFilterRule 过滤规则：命中的失败不计入失败数（不入分母）。
// StatusCodes 与 Keywords 均为可选；同时配置时需同时命中(AND)，多条规则任一命中即命中(OR)。
type ChannelStatusFilterRule struct {
	StatusCodes []int    `json:"status_codes"`
	Keywords    []string `json:"keywords"`
}

// ChannelStatusThresholds 渠道状态分级阈值（成功率百分比）。
type ChannelStatusThresholds struct {
	Healthy  float64 `json:"healthy"`
	Degraded float64 `json:"degraded"`
}

// ChannelStatusConfig 渠道状态统计配置（settings key: channel_status_config）。
type ChannelStatusConfig struct {
	WindowHours    int                       `json:"window_hours"`
	RefreshSeconds int                       `json:"refresh_seconds"`
	FilterRules    []ChannelStatusFilterRule `json:"filter_rules"`
	Thresholds     ChannelStatusThresholds   `json:"thresholds"`
	// DisplayGroupIDs 状态页/公开 API 只展示这些分组（白名单）；空 = 展示全部分组。
	DisplayGroupIDs []int64 `json:"display_group_ids"`
}

const (
	DefaultChannelStatusWindowHours   = 24
	DefaultChannelStatusRefreshSeconds = 60
	DefaultChannelStatusHealthyPct    = 99.0
	DefaultChannelStatusDegradedPct   = 90.0

	maxChannelStatusFilterRules    = 50
	maxChannelStatusStatusCodes    = 20
	maxChannelStatusKeywords       = 50
	maxChannelStatusKeywordLen     = 200
	maxChannelStatusDisplayGroups  = 200
	MaxChannelStatusWindowHours    = 24 * 30
	MinChannelStatusWindowHours    = 1
	MaxChannelStatusRefreshSeconds = 3600
	MinChannelStatusRefreshSeconds = 15
)

func DefaultChannelStatusConfig() ChannelStatusConfig {
	return ChannelStatusConfig{
		WindowHours:    DefaultChannelStatusWindowHours,
		RefreshSeconds: DefaultChannelStatusRefreshSeconds,
		FilterRules:    []ChannelStatusFilterRule{},
		Thresholds:     ChannelStatusThresholds{Healthy: DefaultChannelStatusHealthyPct, Degraded: DefaultChannelStatusDegradedPct},
	}
}

// NormalizeChannelStatusConfig 归一化并校验配置；非法值回落默认/夹取边界。
func NormalizeChannelStatusConfig(raw ChannelStatusConfig) (ChannelStatusConfig, error) {
	cfg := DefaultChannelStatusConfig()

	if raw.WindowHours > 0 {
		cfg.WindowHours = raw.WindowHours
		if cfg.WindowHours > MaxChannelStatusWindowHours {
			return cfg, infraerrors.Newf(400, "INVALID_CHANNEL_STATUS_CONFIG", "window_hours must be <= %d", MaxChannelStatusWindowHours)
		}
		if cfg.WindowHours < MinChannelStatusWindowHours {
			return cfg, infraerrors.Newf(400, "INVALID_CHANNEL_STATUS_CONFIG", "window_hours must be >= %d", MinChannelStatusWindowHours)
		}
	}
	if raw.RefreshSeconds > 0 {
		cfg.RefreshSeconds = raw.RefreshSeconds
		if cfg.RefreshSeconds > MaxChannelStatusRefreshSeconds || cfg.RefreshSeconds < MinChannelStatusRefreshSeconds {
			return cfg, infraerrors.Newf(400, "INVALID_CHANNEL_STATUS_CONFIG", "refresh_seconds must be within [%d,%d]", MinChannelStatusRefreshSeconds, MaxChannelStatusRefreshSeconds)
		}
	}

	if raw.FilterRules != nil {
		if len(raw.FilterRules) > maxChannelStatusFilterRules {
			return cfg, infraerrors.Newf(400, "INVALID_CHANNEL_STATUS_CONFIG", "filter_rules supports at most %d rules", maxChannelStatusFilterRules)
		}
		cfg.FilterRules = make([]ChannelStatusFilterRule, 0, len(raw.FilterRules))
		for i, rule := range raw.FilterRules {
			if len(rule.StatusCodes) > maxChannelStatusStatusCodes {
				return cfg, infraerrors.Newf(400, "INVALID_CHANNEL_STATUS_CONFIG", "filter_rules[%d].status_codes supports at most %d entries", i, maxChannelStatusStatusCodes)
			}
			if len(rule.Keywords) > maxChannelStatusKeywords {
				return cfg, infraerrors.Newf(400, "INVALID_CHANNEL_STATUS_CONFIG", "filter_rules[%d].keywords supports at most %d entries", i, maxChannelStatusKeywords)
			}
			codes := make([]int, 0, len(rule.StatusCodes))
			for _, code := range rule.StatusCodes {
				if code < 100 || code > 599 {
					return cfg, infraerrors.Newf(400, "INVALID_CHANNEL_STATUS_CONFIG", "filter_rules[%d].status_codes contains invalid HTTP status code %d", i, code)
				}
				codes = append(codes, code)
			}
			keywords := make([]string, 0, len(rule.Keywords))
			for _, kw := range rule.Keywords {
				kw = strings.ToLower(strings.TrimSpace(kw))
				if kw == "" {
					continue
				}
				if len(kw) > maxChannelStatusKeywordLen {
					return cfg, infraerrors.Newf(400, "INVALID_CHANNEL_STATUS_CONFIG", "filter_rules[%d].keywords entry too long (max %d chars)", i, maxChannelStatusKeywordLen)
				}
				keywords = append(keywords, kw)
			}
			if len(codes) == 0 && len(keywords) == 0 {
				continue // 空规则跳过（与错误切换策略口径一致）
			}
			cfg.FilterRules = append(cfg.FilterRules, ChannelStatusFilterRule{StatusCodes: codes, Keywords: keywords})
		}
	}

	if raw.Thresholds.Healthy > 0 || raw.Thresholds.Degraded > 0 {
		cfg.Thresholds = raw.Thresholds
	}
	if cfg.Thresholds.Healthy < cfg.Thresholds.Degraded || cfg.Thresholds.Healthy > 100 || cfg.Thresholds.Degraded < 0 {
		return cfg, infraerrors.New(400, "INVALID_CHANNEL_STATUS_CONFIG", "thresholds must satisfy 100 >= healthy >= degraded >= 0")
	}

	if raw.DisplayGroupIDs != nil {
		if len(raw.DisplayGroupIDs) > maxChannelStatusDisplayGroups {
			return cfg, infraerrors.Newf(400, "INVALID_CHANNEL_STATUS_CONFIG", "display_group_ids supports at most %d entries", maxChannelStatusDisplayGroups)
		}
		seen := make(map[int64]struct{}, len(raw.DisplayGroupIDs))
		cfg.DisplayGroupIDs = make([]int64, 0, len(raw.DisplayGroupIDs))
		for _, id := range raw.DisplayGroupIDs {
			if id <= 0 {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			cfg.DisplayGroupIDs = append(cfg.DisplayGroupIDs, id)
		}
	}
	return cfg, nil
}

// ParseChannelStatusConfig 解析 settings 中存储的 JSON 配置；缺失/解析失败回落默认。
func ParseChannelStatusConfig(value string) ChannelStatusConfig {
	cfg := DefaultChannelStatusConfig()
	value = strings.TrimSpace(value)
	if value == "" {
		return cfg
	}
	var raw ChannelStatusConfig
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return cfg
	}
	normalized, err := NormalizeChannelStatusConfig(raw)
	if err != nil {
		return cfg
	}
	return normalized
}

// MatchesChannelStatusFilterRules 判定一条失败记录是否被过滤规则命中（命中=不计入失败）。
// statusCodes 任一匹配（优先看上游状态码，回退客户端状态码）且 keywords 任一命中
// 错误消息（不区分大小写包含）；只有全部配置的维度都满足才视为命中（AND），多条规则 OR。
func MatchesChannelStatusFilterRules(rules []ChannelStatusFilterRule, upstreamStatusCode, clientStatusCode int, errorMessage string) bool {
	if len(rules) == 0 {
		return false
	}
	msgLower := ""
	lowered := false
	for _, rule := range rules {
		statusOK := len(rule.StatusCodes) == 0
		for _, code := range rule.StatusCodes {
			if code == upstreamStatusCode || (upstreamStatusCode == 0 && code == clientStatusCode) {
				statusOK = true
				break
			}
		}
		if !statusOK {
			continue
		}
		if len(rule.Keywords) == 0 {
			return true
		}
		if !lowered {
			msgLower = strings.ToLower(errorMessage)
			lowered = true
		}
		for _, kw := range rule.Keywords {
			if strings.Contains(msgLower, kw) {
				return true
			}
		}
	}
	return false
}

// ---- SettingService 读写（带短缓存，与 image billing thresholds 同款模式） ----

type channelStatusSettingsCacheEntry struct {
	enabled   bool
	config    ChannelStatusConfig
	expiresAt time.Time
}

var (
	channelStatusSettingsMu       sync.RWMutex
	channelStatusSettingsCache    channelStatusSettingsCacheEntry
	channelStatusSettingsCacheErr error
	channelStatusSettingsSF       singleflight.Group
)

// GetChannelStatusEnabledCached 读取渠道状态统计开关（fail-closed：缺失/错误均视为关闭）。
func (s *SettingService) GetChannelStatusEnabledCached(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
	now := time.Now()
	channelStatusSettingsMu.RLock()
	cached := channelStatusSettingsCache
	cacheErr := channelStatusSettingsCacheErr
	channelStatusSettingsMu.RUnlock()
	if cacheErr == nil && now.Before(cached.expiresAt) {
		return cached.enabled
	}

	result, err, _ := channelStatusSettingsSF.Do("channel_status_settings", func() (any, error) {
		channelStatusSettingsMu.RLock()
		if time.Now().Before(channelStatusSettingsCache.expiresAt) {
			channelStatusSettingsMu.RUnlock()
			return channelStatusSettingsCache, nil
		}
		channelStatusSettingsMu.RUnlock()

		values, err := s.settingRepo.GetMultiple(ctx, []string{
			SettingKeyChannelStatusEnabled,
			SettingKeyChannelStatusConfig,
		})
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			return nil, err
		}
		enabled := strings.EqualFold(strings.TrimSpace(values[SettingKeyChannelStatusEnabled]), "true")
		config := ParseChannelStatusConfig(values[SettingKeyChannelStatusConfig])
		entry := channelStatusSettingsCacheEntry{enabled: enabled, config: config, expiresAt: time.Now().Add(channelStatusSettingsCacheTTL)}
		channelStatusSettingsMu.Lock()
		channelStatusSettingsCache = entry
		channelStatusSettingsCacheErr = nil
		channelStatusSettingsMu.Unlock()
		return entry, nil
	})
	if err != nil {
		return false
	}
	entry, _ := result.(channelStatusSettingsCacheEntry)
	return entry.enabled
}

// GetChannelStatusConfigCached 读取渠道状态统计配置（带缓存）。
func (s *SettingService) GetChannelStatusConfigCached(ctx context.Context) ChannelStatusConfig {
	if s == nil || s.settingRepo == nil {
		return DefaultChannelStatusConfig()
	}
	now := time.Now()
	channelStatusSettingsMu.RLock()
	cached := channelStatusSettingsCache
	cacheErr := channelStatusSettingsCacheErr
	channelStatusSettingsMu.RUnlock()
	if cacheErr == nil && now.Before(cached.expiresAt) {
		return cached.config
	}
	// 复用 enabled 的缓存加载（同一批 keys）
	_ = s.GetChannelStatusEnabledCached(ctx)
	channelStatusSettingsMu.RLock()
	defer channelStatusSettingsMu.RUnlock()
	if time.Now().Before(channelStatusSettingsCache.expiresAt) {
		return channelStatusSettingsCache.config
	}
	return DefaultChannelStatusConfig()
}

// GetChannelStatusSettings 读取开关与配置（管理端用，直读存储）。
func (s *SettingService) GetChannelStatusSettings(ctx context.Context) (bool, ChannelStatusConfig, error) {
	if s == nil || s.settingRepo == nil {
		return false, DefaultChannelStatusConfig(), nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyChannelStatusEnabled,
		SettingKeyChannelStatusConfig,
	})
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return false, DefaultChannelStatusConfig(), err
	}
	enabled := strings.EqualFold(strings.TrimSpace(values[SettingKeyChannelStatusEnabled]), "true")
	return enabled, ParseChannelStatusConfig(values[SettingKeyChannelStatusConfig]), nil
}

// SetChannelStatusSettings 保存开关与配置并立刻刷新缓存。
func (s *SettingService) SetChannelStatusSettings(ctx context.Context, enabled bool, config ChannelStatusConfig) (bool, ChannelStatusConfig, error) {
	if s == nil || s.settingRepo == nil {
		return enabled, config, nil
	}
	normalized, err := NormalizeChannelStatusConfig(config)
	if err != nil {
		return enabled, config, err
	}
	configJSON, err := json.Marshal(normalized)
	if err != nil {
		return enabled, config, err
	}
	if err := s.settingRepo.SetMultiple(ctx, map[string]string{
		SettingKeyChannelStatusEnabled: strconv.FormatBool(enabled),
		SettingKeyChannelStatusConfig:  string(configJSON),
	}); err != nil {
		return enabled, config, err
	}
	// 立即刷新缓存，管理端保存后下一轮聚合即按新配置执行
	entry := channelStatusSettingsCacheEntry{enabled: enabled, config: normalized, expiresAt: time.Now().Add(channelStatusSettingsCacheTTL)}
	channelStatusSettingsMu.Lock()
	channelStatusSettingsCache = entry
	channelStatusSettingsCacheErr = nil
	channelStatusSettingsMu.Unlock()
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return enabled, normalized, nil
}
