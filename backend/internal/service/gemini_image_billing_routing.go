package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

func (s *GatewayService) geminiImageBillingForcedAccountIDs(ctx context.Context, groupID *int64, requestedModel string) (accountIDs []int64, tier string, aspectRatio string, ok bool) {
	if s == nil || s.settingService == nil || groupID == nil || *groupID <= 0 || !isImageGenerationModel(requestedModel) {
		return nil, "", "", false
	}
	tier = ImageBillingSchedulingTierFromContext(ctx)
	if tier == "" {
		tier = ImageBillingSize1K
	}
	aspectRatio = ImageBillingSchedulingAspectRatioFromContext(ctx)
	settings := s.settingService.GetGeminiImageBillingRoutingSettingsCached(ctx)
	accountIDs = settings.AccountIDsFor(*groupID, tier, aspectRatio)
	return accountIDs, tier, aspectRatio, len(accountIDs) > 0
}

func (s *GatewayService) selectForcedGeminiImageBillingAccount(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	accountByID map[int64]*Account,
	useMixed bool,
) (*AccountSelectionResult, bool, error) {
	accountIDs, tier, aspectRatio, configured := s.geminiImageBillingForcedAccountIDs(ctx, groupID, requestedModel)
	if !configured {
		return nil, false, nil
	}

	isExcluded := func(accountID int64) bool {
		if excludedIDs == nil {
			return false
		}
		_, excluded := excludedIDs[accountID]
		return excluded
	}
	reasons := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
		}
		if isExcluded(accountID) {
			reasons = append(reasons, fmt.Sprintf("excluded=%d", accountID))
			continue
		}
		account, ok := accountByID[accountID]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("missing=%d", accountID))
			continue
		}
		if !s.isAccountSchedulableForSelection(account) {
			reasons = append(reasons, fmt.Sprintf("schedulable_false=%d", accountID))
			continue
		}
		if !s.isGatewayAccountProfitEligible(ctx, account) {
			reasons = append(reasons, fmt.Sprintf("profit_gate=%d", accountID))
			continue
		}
		if !s.isAccountAllowedForPlatform(account, PlatformGemini, useMixed) {
			reasons = append(reasons, fmt.Sprintf("platform_mismatch=%d:%s", accountID, strings.TrimSpace(account.Platform)))
			continue
		}
		if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, account, requestedModel) {
			reasons = append(reasons, fmt.Sprintf("model_not_supported=%d:%s", accountID, requestedModel))
			continue
		}
		if !s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) {
			reasons = append(reasons, fmt.Sprintf("model_temporarily_limited=%d:%s", accountID, requestedModel))
			continue
		}
		if !s.isAccountSchedulableForQuota(account) {
			reasons = append(reasons, fmt.Sprintf("quota_limited=%d", accountID))
			continue
		}
		if !s.isAccountSchedulableForWindowCost(ctx, account, false) {
			reasons = append(reasons, fmt.Sprintf("window_cost_limited=%d", accountID))
			continue
		}
		if !s.isAccountSchedulableForRPM(ctx, account, false) {
			reasons = append(reasons, fmt.Sprintf("rpm_limited=%d", accountID))
			continue
		}
		result, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if err != nil || result == nil || !result.Acquired {
			reasons = append(reasons, fmt.Sprintf("concurrency_full=%d", accountID))
			continue
		}
		if !s.checkAndRegisterSession(ctx, account, sessionHash) {
			result.ReleaseFunc()
			reasons = append(reasons, fmt.Sprintf("session_limit=%d", accountID))
			continue
		}
		if sessionHash != "" && s.cache != nil {
			_ = s.bindGatewayStickySessionDuringSelection(ctx, groupID, sessionHash, account.ID)
		}
		slog.Debug("gemini image billing forced account selected",
			"group_id", derefGroupID(groupID),
			"tier", tier,
			"aspect_ratio", aspectRatio,
			"account_id", account.ID,
			"configured_account_ids", accountIDs,
		)
		selection, selectErr := s.newSelectionResult(ctx, account, true, result.ReleaseFunc, nil)
		return selection, true, selectErr
	}

	slog.Info("gemini image billing forced accounts exhausted; fallback to default scheduling",
		"group_id", derefGroupID(groupID),
		"tier", tier,
		"aspect_ratio", aspectRatio,
		"configured_account_ids", accountIDs,
		"reasons", reasons,
	)
	return nil, false, nil
}
