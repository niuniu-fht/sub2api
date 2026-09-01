package service

import (
	"context"
	"fmt"
	"log/slog"
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
		if !ok || !s.isAccountSchedulableForSelection(account) {
			reasons = append(reasons, fmt.Sprintf("missing_or_unschedulable=%d", accountID))
			continue
		}
		if !s.isGatewayAccountProfitEligible(ctx, account) ||
			!s.isAccountAllowedForPlatform(account, PlatformGemini, useMixed) ||
			(requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) ||
			!s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) ||
			!s.isAccountSchedulableForQuota(account) ||
			!s.isAccountSchedulableForWindowCost(ctx, account, false) ||
			!s.isAccountSchedulableForRPM(ctx, account, false) {
			reasons = append(reasons, fmt.Sprintf("incompatible=%d", accountID))
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

	return nil, true, fmt.Errorf("%w: gemini_image_billing_forced_%s_%s_accounts_unavailable=%v reasons=%v", ErrNoAvailableAccounts, tier, aspectRatio, accountIDs, reasons)
}
