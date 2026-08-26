package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const imageBillingAreaThresholdSettingsCacheTTL = 30 * time.Second

var imageBillingAreaThresholdSettingsSF singleflight.Group
var imageBillingAccountRoutingSettingsSF singleflight.Group

type cachedImageBillingAreaThresholdSettings struct {
	thresholds ImageBillingAreaThresholds
	expiresAt  int64
}

type cachedImageBillingAccountRoutingSettings struct {
	settings  ImageBillingAccountRoutingSettings
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
}

type ImageBillingAccountRoutingSettings struct {
	Groups map[int64]ImageBillingGroupAccountRouting `json:"groups"`
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
		if len(routing.OneKAccountIDs) == 0 && len(routing.TwoKAccountIDs) == 0 && len(routing.FourKAccountIDs) == 0 {
			continue
		}
		normalized.Groups[groupID] = routing
	}
	return normalized
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
