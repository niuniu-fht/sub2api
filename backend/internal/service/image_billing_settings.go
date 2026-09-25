package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const imageBillingAreaThresholdSettingsCacheTTL = 30 * time.Second

var imageBillingAreaThresholdSettingsSF singleflight.Group
var imageBillingAccountRoutingSettingsSF singleflight.Group
var geminiImageBillingRoutingSettingsSF singleflight.Group

type cachedImageBillingAreaThresholdSettings struct {
	thresholds ImageBillingAreaThresholds
	expiresAt  int64
}

type cachedImageBillingAccountRoutingSettings struct {
	settings  ImageBillingAccountRoutingSettings
	expiresAt int64
}

type cachedGeminiImageBillingRoutingSettings struct {
	settings  GeminiImageBillingRoutingSettings
	expiresAt int64
}

type ImageBillingGroupAccountRouting struct {
	// AccountIDs fields are the current multi-account configuration.
	// AccountID fields are kept for backward compatibility with old saved JSON.
	OneKAccountIDs  []int64 `json:"one_k_account_ids,omitempty"`
	TwoKAccountIDs  []int64 `json:"two_k_account_ids,omitempty"`
	FourKAccountIDs []int64 `json:"four_k_account_ids,omitempty"`
	OneKAccountID   int64   `json:"one_k_account_id,omitempty"`
	TwoKAccountID   int64   `json:"two_k_account_id,omitempty"`
	FourKAccountID  int64   `json:"four_k_account_id,omitempty"`
	Rules           []OpenAIImageBillingRoutingRule `json:"rules,omitempty"`
	// TierModes 兜底账号链(仅按档位匹配)的调度方式:
	// "1K"/"2K"/"4K" -> "priority"(默认,按顺序)/"round_robin"(逐请求轮转)。
	TierModes map[string]string `json:"tier_modes,omitempty"`
}

type ImageBillingAccountRoutingSettings struct {
	Groups map[int64]ImageBillingGroupAccountRouting `json:"groups"`
}

const (
	ImageBillingRoutingModePriority   = "priority"
	ImageBillingRoutingModeRoundRobin = "round_robin"
	// ImageQualityUnmatched 伪质量:规则声明此质量时,命中"请求 quality 未匹配
	// 任何具体质量规则"的场景(含请求未携带 quality/auto/未知值),承担原兜底链角色。
	ImageQualityUnmatched = "unmatched"
)

// normalizeImageBillingRuleQuality 规则侧质量归一化:具体质量照常;
// 规则允许声明 unmatched(承担兜底角色);其余未知值视为无效规则丢弃。
func normalizeImageBillingRuleQuality(quality string) string {
	q := strings.ToLower(strings.TrimSpace(quality))
	if q == ImageQualityUnmatched {
		return ImageQualityUnmatched
	}
	return NormalizeOpenAIImageQuality(q)
}

type OpenAIImageBillingRoutingRule struct {
	Quality    string  `json:"quality"`
	Tier       string  `json:"tier"`
	Mode       string  `json:"mode,omitempty"`
	AccountIDs []int64 `json:"account_ids"`
	// ImageCounts 参考图数量约束(空=不限)。请求携带的参考图张数在列表内才命中,
	// 用于把请求路由到只支持特定参考图数量的号池。
	ImageCounts []int `json:"image_counts,omitempty"`
}

const GeminiImageBillingAspectRatioAny = "*"

type GeminiImageBillingRoutingRule struct {
	Tier        string  `json:"tier"`
	AspectRatio string  `json:"aspect_ratio"`
	AccountIDs  []int64 `json:"account_ids"`
}

type GeminiImageBillingGroupRouting struct {
	Rules []GeminiImageBillingRoutingRule `json:"rules"`
}

type GeminiImageBillingRoutingSettings struct {
	Groups map[int64]GeminiImageBillingGroupRouting `json:"groups"`
}

func NormalizeImageBillingAccountRoutingSettings(settings ImageBillingAccountRoutingSettings) ImageBillingAccountRoutingSettings {
	normalized := ImageBillingAccountRoutingSettings{Groups: map[int64]ImageBillingGroupAccountRouting{}}
	for groupID, routing := range settings.Groups {
		if groupID <= 0 {
			continue
		}
		routing.OneKAccountIDs = normalizeImageBillingRoutingAccountIDs(routing.OneKAccountIDs, routing.OneKAccountID)
		routing.TwoKAccountIDs = normalizeImageBillingRoutingAccountIDs(routing.TwoKAccountIDs, routing.TwoKAccountID)
		routing.FourKAccountIDs = normalizeImageBillingRoutingAccountIDs(routing.FourKAccountIDs, routing.FourKAccountID)
		// Store only the multi-account shape after normalization; old single fields remain accepted on input.
		routing.OneKAccountID = 0
		routing.TwoKAccountID = 0
		routing.FourKAccountID = 0
		routing.Rules = normalizeOpenAIImageBillingRoutingRules(routing.Rules)
		routing.TierModes = normalizeImageBillingTierModes(routing.TierModes)
		if len(routing.OneKAccountIDs) == 0 && len(routing.TwoKAccountIDs) == 0 && len(routing.FourKAccountIDs) == 0 && len(routing.Rules) == 0 {
			continue
		}
		normalized.Groups[groupID] = routing
	}
	return normalized
}

func normalizeOpenAIImageBillingRoutingRules(rules []OpenAIImageBillingRoutingRule) []OpenAIImageBillingRoutingRule {
	if len(rules) == 0 {
		return nil
	}
	merged := make(map[string]*OpenAIImageBillingRoutingRule, len(rules))
	order := make([]string, 0, len(rules))
	for _, rule := range rules {
		quality := normalizeImageBillingRuleQuality(rule.Quality)
		tier := NormalizeImageBillingTierOrDefault(rule.Tier)
		ids := normalizeImageBillingRoutingAccountIDs(rule.AccountIDs, 0)
		if quality == "" || tier == "" || len(ids) == 0 {
			continue
		}
		counts := normalizeImageBillingRuleImageCounts(rule.ImageCounts)
		mode := normalizeImageBillingRoutingMode(rule.Mode)
		countKey := "any"
		if len(counts) > 0 {
			parts := make([]string, 0, len(counts))
			for _, c := range counts {
				parts = append(parts, fmt.Sprintf("%d", c))
			}
			countKey = strings.Join(parts, "+")
		}
		key := quality + "|" + tier + "|" + countKey
		if existing, ok := merged[key]; ok {
			existing.AccountIDs = normalizeImageBillingRoutingAccountIDs(append(existing.AccountIDs, ids...), 0)
			if existing.Mode == "" && mode != "" {
				existing.Mode = mode
			}
			continue
		}
		copied := rule
		copied.Quality = quality
		copied.Tier = tier
		copied.Mode = mode
		copied.AccountIDs = ids
		merged[key] = &copied
		order = append(order, key)
	}
	out := make([]OpenAIImageBillingRoutingRule, 0, len(order))
	for _, key := range order {
		if rule := merged[key]; rule != nil {
			out = append(out, *rule)
		}
	}
	return out
}

func normalizeImageBillingRoutingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ImageBillingRoutingModeRoundRobin:
		return ImageBillingRoutingModeRoundRobin
	default:
		return ImageBillingRoutingModePriority
	}
}

func normalizeImageBillingRoutingAccountIDs(ids []int64, legacyID int64) []int64 {
	seen := make(map[int64]struct{}, len(ids)+1)
	out := make([]int64, 0, len(ids)+1)
	appendID := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range ids {
		appendID(id)
	}
	appendID(legacyID)
	return out
}

func (s ImageBillingAccountRoutingSettings) AccountIDsFor(groupID int64, tier string) []int64 {
	if groupID <= 0 {
		return nil
	}
	routing, ok := s.Groups[groupID]
	if !ok {
		return nil
	}
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case ImageBillingSize1K:
		return append([]int64(nil), routing.OneKAccountIDs...)
	case ImageBillingSize2K:
		return append([]int64(nil), routing.TwoKAccountIDs...)
	case ImageBillingSize4K:
		return append([]int64(nil), routing.FourKAccountIDs...)
	default:
		return nil
	}
}

func (s ImageBillingAccountRoutingSettings) AccountIDFor(groupID int64, tier string) int64 {
	ids := s.AccountIDsFor(groupID, tier)
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func normalizeImageBillingRuleImageCounts(counts []int) []int {
	if len(counts) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(counts))
	out := make([]int, 0, len(counts))
	for _, c := range counts {
		if c < 1 || c > 32 {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// qualityRoutingRule 按 quality+tier 查找规则;参考图数量感知:
// 优先命中声明了参考图数量且包含请求数量的规则(更具体),否则回退到不限数量的规则。
func (s ImageBillingAccountRoutingSettings) qualityRoutingRule(groupID int64, quality string, tier string, imageCount int) *OpenAIImageBillingRoutingRule {
	if groupID <= 0 {
		return nil
	}
	routing, ok := s.Groups[groupID]
	if !ok {
		return nil
	}
	requestQuality := NormalizeOpenAIImageQuality(quality)
	tier = NormalizeImageBillingTierOrDefault(tier)
	if tier == "" {
		return nil
	}
	// 匹配目标:请求质量可识别则按具体值匹配;为空/auto/未知时匹配 unmatched 规则。
	quality = requestQuality
	if quality == "" {
		quality = ImageQualityUnmatched
	}
	var generic *OpenAIImageBillingRoutingRule
	for i := range routing.Rules {
		rule := &routing.Rules[i]
		if rule.Quality != quality || rule.Tier != tier || len(rule.AccountIDs) == 0 {
			continue
		}
		if len(rule.ImageCounts) > 0 {
			for _, c := range rule.ImageCounts {
				if c == imageCount {
					return rule
				}
			}
			continue
		}
		if generic == nil {
			generic = rule
		}
	}
	return generic
}

func (s ImageBillingAccountRoutingSettings) AccountIDsForQuality(groupID int64, quality string, tier string, imageCount int) []int64 {
	if rule := s.qualityRoutingRule(groupID, quality, tier, imageCount); rule != nil {
		return append([]int64(nil), rule.AccountIDs...)
	}
	// 未配置 quality×size 时回退到旧 size-only 路由。
	return s.AccountIDsFor(groupID, tier)
}

// AccountIDsAndModeFor 返回命中规则的账号列表与调度方式:
// ① 具体 quality 规则 → ② unmatched(不匹配)规则(原兜底角色) → ③ 旧档位兜底字段(兼容历史数据)。
func (s ImageBillingAccountRoutingSettings) AccountIDsAndModeFor(groupID int64, quality string, tier string, imageCount int) ([]int64, string) {
	if rule := s.qualityRoutingRule(groupID, quality, tier, imageCount); rule != nil {
		return append([]int64(nil), rule.AccountIDs...), normalizeImageBillingRoutingMode(rule.Mode)
	}
	if rule := s.qualityRoutingRule(groupID, ImageQualityUnmatched, tier, imageCount); rule != nil {
		return append([]int64(nil), rule.AccountIDs...), normalizeImageBillingRoutingMode(rule.Mode)
	}
	if legacy := s.AccountIDsFor(groupID, tier); len(legacy) > 0 {
		return legacy, ImageBillingRoutingModePriority
	}
	return nil, ImageBillingRoutingModePriority
}

func (s ImageBillingAccountRoutingSettings) RoutingModeForQuality(groupID int64, quality string, tier string, imageCount int) string {
	if rule := s.qualityRoutingRule(groupID, quality, tier, imageCount); rule != nil {
		return normalizeImageBillingRoutingMode(rule.Mode)
	}
	return s.RoutingModeForTier(groupID, tier)
}

// RoutingModeForTier 返回兜底账号链(仅按档位匹配)的调度方式,默认 priority。
func (s ImageBillingAccountRoutingSettings) RoutingModeForTier(groupID int64, tier string) string {
	if groupID <= 0 {
		return ImageBillingRoutingModePriority
	}
	routing, ok := s.Groups[groupID]
	if !ok {
		return ImageBillingRoutingModePriority
	}
	return normalizeImageBillingRoutingMode(routing.TierModes[strings.ToUpper(strings.TrimSpace(tier))])
}

func normalizeImageBillingTierModes(modes map[string]string) map[string]string {
	if len(modes) == 0 {
		return nil
	}
	out := make(map[string]string, len(modes))
	for tier, mode := range modes {
		normalizedTier := NormalizeImageBillingTierOrDefault(tier)
		if normalizedTier == "" {
			continue
		}
		out[normalizedTier] = normalizeImageBillingRoutingMode(mode)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NormalizeGeminiImageBillingAspectRatio(aspectRatio string) string {
	aspectRatio = strings.ToLower(strings.TrimSpace(aspectRatio))
	aspectRatio = strings.ReplaceAll(aspectRatio, " ", "")
	switch aspectRatio {
	case "", "auto":
		return ""
	case "*", "all", "any", "全部", "全部比例":
		return GeminiImageBillingAspectRatioAny
	case "1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "21:9":
		return aspectRatio
	default:
		return aspectRatio
	}
}

func NormalizeGeminiImageBillingRoutingSettings(settings GeminiImageBillingRoutingSettings) GeminiImageBillingRoutingSettings {
	normalized := GeminiImageBillingRoutingSettings{Groups: map[int64]GeminiImageBillingGroupRouting{}}
	for groupID, routing := range settings.Groups {
		if groupID <= 0 {
			continue
		}
		rules := make([]GeminiImageBillingRoutingRule, 0, len(routing.Rules))
		seenRules := make(map[string]struct{}, len(routing.Rules))
		for _, rule := range routing.Rules {
			tier := NormalizeImageBillingTierOrDefault(rule.Tier)
			aspectRatio := NormalizeGeminiImageBillingAspectRatio(rule.AspectRatio)
			if aspectRatio == "" {
				aspectRatio = GeminiImageBillingAspectRatioAny
			}
			accountIDs := normalizeImageBillingRoutingAccountIDs(rule.AccountIDs, 0)
			if len(accountIDs) == 0 {
				continue
			}
			key := tier + "|" + aspectRatio
			if _, ok := seenRules[key]; ok {
				for i := range rules {
					if rules[i].Tier == tier && rules[i].AspectRatio == aspectRatio {
						rules[i].AccountIDs = normalizeImageBillingRoutingAccountIDs(append(rules[i].AccountIDs, accountIDs...), 0)
						break
					}
				}
				continue
			}
			seenRules[key] = struct{}{}
			rules = append(rules, GeminiImageBillingRoutingRule{
				Tier:        tier,
				AspectRatio: aspectRatio,
				AccountIDs:  accountIDs,
			})
		}
		if len(rules) > 0 {
			normalized.Groups[groupID] = GeminiImageBillingGroupRouting{Rules: rules}
		}
	}
	return normalized
}

func (s GeminiImageBillingRoutingSettings) AccountIDsFor(groupID int64, tier string, aspectRatio string) []int64 {
	if groupID <= 0 {
		return nil
	}
	routing, ok := s.Groups[groupID]
	if !ok {
		return nil
	}
	tier = NormalizeImageBillingTierOrDefault(tier)
	aspectRatio = NormalizeGeminiImageBillingAspectRatio(aspectRatio)
	for _, rule := range routing.Rules {
		if rule.Tier == tier && rule.AspectRatio == aspectRatio && len(rule.AccountIDs) > 0 {
			return append([]int64(nil), rule.AccountIDs...)
		}
	}
	for _, rule := range routing.Rules {
		if rule.Tier == tier && rule.AspectRatio == GeminiImageBillingAspectRatioAny && len(rule.AccountIDs) > 0 {
			return append([]int64(nil), rule.AccountIDs...)
		}
	}
	return nil
}

func (s GeminiImageBillingRoutingSettings) AccountAllowedFor(groupID int64, accountID int64, tier string, aspectRatio string) bool {
	if groupID <= 0 || accountID <= 0 {
		return true
	}
	routing, ok := s.Groups[groupID]
	if !ok {
		return true
	}
	tier = NormalizeImageBillingTierOrDefault(tier)
	aspectRatio = NormalizeGeminiImageBillingAspectRatio(aspectRatio)

	constrained := false
	for _, rule := range routing.Rules {
		if !geminiImageBillingRuleContainsAccount(rule, accountID) {
			continue
		}
		constrained = true
		if rule.Tier == tier && (rule.AspectRatio == aspectRatio || rule.AspectRatio == GeminiImageBillingAspectRatioAny) {
			return true
		}
	}
	return !constrained
}

func (s GeminiImageBillingRoutingSettings) ConstrainedButNotAllowedAccountIDsFor(groupID int64, tier string, aspectRatio string) []int64 {
	if groupID <= 0 {
		return nil
	}
	routing, ok := s.Groups[groupID]
	if !ok {
		return nil
	}
	seen := make(map[int64]struct{})
	ids := make([]int64, 0)
	for _, rule := range routing.Rules {
		for _, accountID := range rule.AccountIDs {
			if accountID <= 0 {
				continue
			}
			if _, exists := seen[accountID]; exists {
				continue
			}
			seen[accountID] = struct{}{}
			if !s.AccountAllowedFor(groupID, accountID, tier, aspectRatio) {
				ids = append(ids, accountID)
			}
		}
	}
	return ids
}

func geminiImageBillingRuleContainsAccount(rule GeminiImageBillingRoutingRule, accountID int64) bool {
	for _, id := range rule.AccountIDs {
		if id == accountID {
			return true
		}
	}
	return false
}

// GetImageBillingAreaThresholdSettings reads image billing thresholds from storage.
// Missing or invalid values fall back to the built-in defaults.
func (s *SettingService) GetImageBillingAreaThresholdSettings(ctx context.Context) (ImageBillingAreaThresholds, error) {
	thresholds := DefaultImageBillingAreaThresholds()
	if s == nil || s.settingRepo == nil {
		return SetImageBillingAreaThresholds(thresholds.TwoKPixelThreshold, thresholds.FourKPixelThreshold), nil
	}

	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyImageBilling2KPixelThreshold,
		SettingKeyImageBilling4KPixelThreshold,
	})
	if err != nil {
		if !errors.Is(err, ErrSettingNotFound) {
			return thresholds, err
		}
		values = map[string]string{}
	}

	thresholds = imageBillingAreaThresholdsFromSettings(values)
	return s.storeImageBillingAreaThresholdSettingsCache(thresholds, imageBillingAreaThresholdSettingsCacheTTL), nil
}

// SetImageBillingAreaThresholdSettings saves image billing thresholds and refreshes the runtime cache immediately.
func (s *SettingService) SetImageBillingAreaThresholdSettings(ctx context.Context, thresholds ImageBillingAreaThresholds) (ImageBillingAreaThresholds, error) {
	thresholds = NormalizeImageBillingAreaThresholds(thresholds.TwoKPixelThreshold, thresholds.FourKPixelThreshold)
	if s == nil || s.settingRepo == nil {
		return SetImageBillingAreaThresholds(thresholds.TwoKPixelThreshold, thresholds.FourKPixelThreshold), nil
	}

	updates := map[string]string{
		SettingKeyImageBilling2KPixelThreshold: strconv.FormatInt(thresholds.TwoKPixelThreshold, 10),
		SettingKeyImageBilling4KPixelThreshold: strconv.FormatInt(thresholds.FourKPixelThreshold, 10),
	}
	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return thresholds, err
	}
	thresholds = s.storeImageBillingAreaThresholdSettingsCache(thresholds, imageBillingAreaThresholdSettingsCacheTTL)
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return thresholds, nil
}

// GetImageBillingAreaThresholdSettingsCached returns thresholds for hot-path billing classification.
func (s *SettingService) GetImageBillingAreaThresholdSettingsCached(ctx context.Context) ImageBillingAreaThresholds {
	if s == nil || s.settingRepo == nil {
		return CurrentImageBillingAreaThresholds()
	}
	now := time.Now()
	if cached, ok := imageBillingAreaThresholdSettingsStoreLoad(); ok && now.UnixNano() < cached.expiresAt {
		return cached.thresholds
	}

	result, err, _ := imageBillingAreaThresholdSettingsSF.Do("image_billing_area_thresholds", func() (any, error) {
		if cached, ok := imageBillingAreaThresholdSettingsStoreLoad(); ok && time.Now().UnixNano() < cached.expiresAt {
			return cached.thresholds, nil
		}
		return s.GetImageBillingAreaThresholdSettings(ctx)
	})
	if err == nil {
		if thresholds, ok := result.(ImageBillingAreaThresholds); ok {
			return thresholds
		}
	}
	return CurrentImageBillingAreaThresholds()
}

func imageBillingAreaThresholdsFromSettings(values map[string]string) ImageBillingAreaThresholds {
	defaults := DefaultImageBillingAreaThresholds()
	twoK := defaults.TwoKPixelThreshold
	fourK := defaults.FourKPixelThreshold
	if raw := values[SettingKeyImageBilling2KPixelThreshold]; raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			twoK = parsed
		}
	}
	if raw := values[SettingKeyImageBilling4KPixelThreshold]; raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			fourK = parsed
		}
	}
	return NormalizeImageBillingAreaThresholds(twoK, fourK)
}

func (s *SettingService) storeImageBillingAreaThresholdSettingsCache(thresholds ImageBillingAreaThresholds, ttl time.Duration) ImageBillingAreaThresholds {
	thresholds = SetImageBillingAreaThresholds(thresholds.TwoKPixelThreshold, thresholds.FourKPixelThreshold)
	imageBillingAreaThresholdsStore.Store(thresholds)
	imageBillingAreaThresholdSettingsCache.Store(&cachedImageBillingAreaThresholdSettings{
		thresholds: thresholds,
		expiresAt:  time.Now().Add(ttl).UnixNano(),
	})
	return thresholds
}

var imageBillingAreaThresholdSettingsCache atomic.Value

func imageBillingAreaThresholdSettingsStoreLoad() (*cachedImageBillingAreaThresholdSettings, bool) {
	cached, ok := imageBillingAreaThresholdSettingsCache.Load().(*cachedImageBillingAreaThresholdSettings)
	return cached, ok && cached != nil
}

// GetImageBillingAccountRoutingSettings reads per-group image tier account routing from storage.
func (s *SettingService) GetImageBillingAccountRoutingSettings(ctx context.Context) (ImageBillingAccountRoutingSettings, error) {
	settings := ImageBillingAccountRoutingSettings{Groups: map[int64]ImageBillingGroupAccountRouting{}}
	if s == nil || s.settingRepo == nil {
		return settings, nil
	}
	setting, err := s.settingRepo.Get(ctx, SettingKeyImageBillingAccountRouting)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return s.storeImageBillingAccountRoutingSettingsCache(settings, imageBillingAreaThresholdSettingsCacheTTL), nil
		}
		return settings, err
	}
	raw := ""
	if setting != nil {
		raw = setting.Value
	}
	settings = imageBillingAccountRoutingSettingsFromString(raw)
	return s.storeImageBillingAccountRoutingSettingsCache(settings, imageBillingAreaThresholdSettingsCacheTTL), nil
}

// SetImageBillingAccountRoutingSettings saves per-group image tier account routing.
func (s *SettingService) SetImageBillingAccountRoutingSettings(ctx context.Context, settings ImageBillingAccountRoutingSettings) (ImageBillingAccountRoutingSettings, error) {
	settings = NormalizeImageBillingAccountRoutingSettings(settings)
	if s == nil || s.settingRepo == nil {
		return settings, nil
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return settings, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyImageBillingAccountRouting, string(raw)); err != nil {
		return settings, err
	}
	settings = s.storeImageBillingAccountRoutingSettingsCache(settings, imageBillingAreaThresholdSettingsCacheTTL)
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return settings, nil
}

// GetImageBillingAccountRoutingSettingsCached returns cached routing for scheduling hot paths.
func (s *SettingService) GetImageBillingAccountRoutingSettingsCached(ctx context.Context) ImageBillingAccountRoutingSettings {
	empty := ImageBillingAccountRoutingSettings{Groups: map[int64]ImageBillingGroupAccountRouting{}}
	if s == nil || s.settingRepo == nil {
		return empty
	}
	now := time.Now()
	if cached, ok := imageBillingAccountRoutingSettingsStoreLoad(); ok && now.UnixNano() < cached.expiresAt {
		return cached.settings
	}

	result, err, _ := imageBillingAccountRoutingSettingsSF.Do("image_billing_account_routing", func() (any, error) {
		if cached, ok := imageBillingAccountRoutingSettingsStoreLoad(); ok && time.Now().UnixNano() < cached.expiresAt {
			return cached.settings, nil
		}
		return s.GetImageBillingAccountRoutingSettings(ctx)
	})
	if err == nil {
		if settings, ok := result.(ImageBillingAccountRoutingSettings); ok {
			return settings
		}
	}
	return empty
}

func imageBillingAccountRoutingSettingsFromString(raw string) ImageBillingAccountRoutingSettings {
	settings := ImageBillingAccountRoutingSettings{Groups: map[int64]ImageBillingGroupAccountRouting{}}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return settings
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		// Backward-compatible shorthand: {"1":{"two_k_account_id":2,"four_k_account_id":3}}
		var groups map[int64]ImageBillingGroupAccountRouting
		if err2 := json.Unmarshal([]byte(raw), &groups); err2 == nil {
			settings.Groups = groups
		}
	}
	if settings.Groups == nil {
		settings.Groups = map[int64]ImageBillingGroupAccountRouting{}
	}
	return NormalizeImageBillingAccountRoutingSettings(settings)
}

func (s *SettingService) storeImageBillingAccountRoutingSettingsCache(settings ImageBillingAccountRoutingSettings, ttl time.Duration) ImageBillingAccountRoutingSettings {
	settings = NormalizeImageBillingAccountRoutingSettings(settings)
	imageBillingAccountRoutingSettingsCache.Store(&cachedImageBillingAccountRoutingSettings{
		settings:  settings,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
	return settings
}

var imageBillingAccountRoutingSettingsCache atomic.Value

func imageBillingAccountRoutingSettingsStoreLoad() (*cachedImageBillingAccountRoutingSettings, bool) {
	cached, ok := imageBillingAccountRoutingSettingsCache.Load().(*cachedImageBillingAccountRoutingSettings)
	return cached, ok && cached != nil
}

// GetGeminiImageBillingRoutingSettings reads Gemini per-group/tier/aspect account routing from storage.
func (s *SettingService) GetGeminiImageBillingRoutingSettings(ctx context.Context) (GeminiImageBillingRoutingSettings, error) {
	settings := GeminiImageBillingRoutingSettings{Groups: map[int64]GeminiImageBillingGroupRouting{}}
	if s == nil || s.settingRepo == nil {
		return settings, nil
	}
	setting, err := s.settingRepo.Get(ctx, SettingKeyGeminiImageBillingRouting)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return s.storeGeminiImageBillingRoutingSettingsCache(settings, imageBillingAreaThresholdSettingsCacheTTL), nil
		}
		return settings, err
	}
	raw := ""
	if setting != nil {
		raw = setting.Value
	}
	settings = geminiImageBillingRoutingSettingsFromString(raw)
	return s.storeGeminiImageBillingRoutingSettingsCache(settings, imageBillingAreaThresholdSettingsCacheTTL), nil
}

// SetGeminiImageBillingRoutingSettings saves Gemini per-group/tier/aspect account routing.
func (s *SettingService) SetGeminiImageBillingRoutingSettings(ctx context.Context, settings GeminiImageBillingRoutingSettings) (GeminiImageBillingRoutingSettings, error) {
	settings = NormalizeGeminiImageBillingRoutingSettings(settings)
	if s == nil || s.settingRepo == nil {
		return settings, nil
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return settings, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyGeminiImageBillingRouting, string(raw)); err != nil {
		return settings, err
	}
	settings = s.storeGeminiImageBillingRoutingSettingsCache(settings, imageBillingAreaThresholdSettingsCacheTTL)
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return settings, nil
}

// GetGeminiImageBillingRoutingSettingsCached returns cached Gemini routing for scheduling hot paths.
func (s *SettingService) GetGeminiImageBillingRoutingSettingsCached(ctx context.Context) GeminiImageBillingRoutingSettings {
	empty := GeminiImageBillingRoutingSettings{Groups: map[int64]GeminiImageBillingGroupRouting{}}
	if s == nil || s.settingRepo == nil {
		return empty
	}
	now := time.Now()
	if cached, ok := geminiImageBillingRoutingSettingsStoreLoad(); ok && now.UnixNano() < cached.expiresAt {
		return cached.settings
	}

	result, err, _ := geminiImageBillingRoutingSettingsSF.Do("gemini_image_billing_routing", func() (any, error) {
		if cached, ok := geminiImageBillingRoutingSettingsStoreLoad(); ok && time.Now().UnixNano() < cached.expiresAt {
			return cached.settings, nil
		}
		return s.GetGeminiImageBillingRoutingSettings(ctx)
	})
	if err == nil {
		if settings, ok := result.(GeminiImageBillingRoutingSettings); ok {
			return settings
		}
	}
	return empty
}

func geminiImageBillingRoutingSettingsFromString(raw string) GeminiImageBillingRoutingSettings {
	settings := GeminiImageBillingRoutingSettings{Groups: map[int64]GeminiImageBillingGroupRouting{}}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return settings
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		var groups map[int64]GeminiImageBillingGroupRouting
		if err2 := json.Unmarshal([]byte(raw), &groups); err2 == nil {
			settings.Groups = groups
		}
	}
	if settings.Groups == nil {
		settings.Groups = map[int64]GeminiImageBillingGroupRouting{}
	}
	return NormalizeGeminiImageBillingRoutingSettings(settings)
}

func (s *SettingService) storeGeminiImageBillingRoutingSettingsCache(settings GeminiImageBillingRoutingSettings, ttl time.Duration) GeminiImageBillingRoutingSettings {
	settings = NormalizeGeminiImageBillingRoutingSettings(settings)
	geminiImageBillingRoutingSettingsCache.Store(&cachedGeminiImageBillingRoutingSettings{
		settings:  settings,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
	return settings
}

var geminiImageBillingRoutingSettingsCache atomic.Value

func geminiImageBillingRoutingSettingsStoreLoad() (*cachedGeminiImageBillingRoutingSettings, bool) {
	cached, ok := geminiImageBillingRoutingSettingsCache.Load().(*cachedGeminiImageBillingRoutingSettings)
	return cached, ok && cached != nil
}
