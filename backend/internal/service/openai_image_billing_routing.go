package service

import (
	"context"
	"fmt"
	"log/slog"
)

const openAIAccountScheduleLayerImageBillingRouting = "image_billing_routing"

func (s *OpenAIGatewayService) imageBillingForcedAccountID(ctx context.Context, groupID *int64) (accountID int64, tier string, ok bool) {
	if s == nil || s.settingService == nil || groupID == nil || *groupID <= 0 {
		return 0, "", false
	}
	tier = ImageBillingSchedulingTierFromContext(ctx)
	if tier == "" {
		return 0, "", false
	}
	settings := s.settingService.GetImageBillingAccountRoutingSettingsCached(ctx)
	accountID = settings.AccountIDFor(*groupID, tier)
	return accountID, tier, accountID > 0
}

func (s *OpenAIGatewayService) selectForcedOpenAIImageBillingAccount(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, bool, error) {
	accountID, tier, configured := s.imageBillingForcedAccountID(ctx, req.GroupID)
	if !configured {
		return nil, false, nil
	}
	if req.ExcludedIDs != nil {
		if _, excluded := req.ExcludedIDs[accountID]; excluded {
			return nil, true, noAvailableOpenAISelectionError(req.RequestedModel, req.RequireCompact, fmt.Sprintf("image_billing_forced_%s_account_excluded=%d", tier, accountID))
		}
	}

	account, err := s.getSchedulableAccount(ctx, accountID)
	if err != nil || account == nil {
		return nil, true, noAvailableOpenAISelectionError(req.RequestedModel, req.RequireCompact, fmt.Sprintf("image_billing_forced_%s_account_not_schedulable=%d", tier, accountID))
	}
	platform := normalizeOpenAICompatiblePlatform(req.Platform)
	if account.Platform != platform || !account.IsOpenAICompatible() || !account.IsSchedulable() {
		return nil, true, noAvailableOpenAISelectionError(req.RequestedModel, req.RequireCompact, fmt.Sprintf("image_billing_forced_%s_account_platform_or_status_mismatch=%d", tier, accountID))
	}
	if !s.openAIAccountMatchesSchedulingGroup(account, req.GroupID) {
		return nil, true, noAvailableOpenAISelectionError(req.RequestedModel, req.RequireCompact, fmt.Sprintf("image_billing_forced_%s_account_not_in_group=%d", tier, accountID))
	}
	if shouldClearStickySession(account, req.RequestedModel) ||
		!isOpenAICompatibleAccountEligibleForRequest(ctx, account, platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability) ||
		!accountSupportsOpenAICapabilities(account, req.RequiredCapability, req.RequiredImageCapability) ||
		!s.isOpenAIAccountTransportCompatible(account, req.RequiredTransport) ||
		s.isOpenAIAccountRequestRuntimeBlocked(account, req.RequestedModel) ||
		!parentHealthyForShadow(account, s.parentAccountLookup(ctx)) {
		return nil, true, noAvailableOpenAISelectionError(req.RequestedModel, req.RequireCompact, fmt.Sprintf("image_billing_forced_%s_account_incompatible=%d", tier, accountID))
	}
	if req.GroupID != nil && s.needsUpstreamChannelRestrictionCheck(ctx, req.GroupID) &&
		s.isUpstreamModelRestrictedByChannel(ctx, *req.GroupID, account, req.RequestedModel, req.RequireCompact) {
		return nil, true, noAvailableOpenAISelectionError(req.RequestedModel, req.RequireCompact, fmt.Sprintf("image_billing_forced_%s_account_channel_restricted=%d", tier, accountID))
	}

	account = s.recheckSelectedOpenAIAccountFromDB(ctx, account, req.GroupID, platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
	if account == nil ||
		!s.openAIAccountMatchesSchedulingGroup(account, req.GroupID) ||
		!accountSupportsOpenAICapabilities(account, req.RequiredCapability, req.RequiredImageCapability) ||
		!s.isOpenAIAccountTransportCompatible(account, req.RequiredTransport) ||
		!parentHealthyForShadow(account, s.parentAccountLookup(ctx)) {
		return nil, true, noAvailableOpenAISelectionError(req.RequestedModel, req.RequireCompact, fmt.Sprintf("image_billing_forced_%s_account_recheck_failed=%d", tier, accountID))
	}

	if result, acquireErr := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency); acquireErr == nil && result != nil && result.Acquired {
		slog.Debug("openai image billing forced account selected",
			"group_id", derefGroupID(req.GroupID),
			"tier", tier,
			"account_id", account.ID,
			"acquired", true,
		)
		selection, selectErr := s.newAcquiredSelectionResult(ctx, account, result.ReleaseFunc)
		return selection, true, selectErr
	}

	cfg := s.schedulingConfig()
	slog.Debug("openai image billing forced account selected with wait plan",
		"group_id", derefGroupID(req.GroupID),
		"tier", tier,
		"account_id", account.ID,
	)
	selection, selectErr := s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
		AccountID:      account.ID,
		MaxConcurrency: account.Concurrency,
		Timeout:        cfg.FallbackWaitTimeout,
		MaxWaiting:     cfg.FallbackMaxWaiting,
	})
	return selection, true, selectErr
}
