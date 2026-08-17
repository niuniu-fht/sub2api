package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
)

const openAIAccountScheduleLayerImageBillingRouting = "image_billing_routing"

var openAIImageBillingRoutingCounters sync.Map

func (s *OpenAIGatewayService) imageBillingForcedAccountIDs(ctx context.Context, groupID *int64) (accountIDs []int64, tier string, ok bool) {
	if s == nil || s.settingService == nil || groupID == nil || *groupID <= 0 {
		return nil, "", false
	}
	tier = ImageBillingSchedulingTierFromContext(ctx)
	if tier == "" {
		return nil, "", false
	}
	settings := s.settingService.GetImageBillingAccountRoutingSettingsCached(ctx)
	accountIDs = rotateOpenAIImageBillingRoutingAccountIDs(*groupID, tier, settings.AccountIDsFor(*groupID, tier))
	return accountIDs, tier, len(accountIDs) > 0
}

func rotateOpenAIImageBillingRoutingAccountIDs(groupID int64, tier string, accountIDs []int64) []int64 {
	if len(accountIDs) <= 1 {
		return append([]int64(nil), accountIDs...)
	}
	keyBuilder := strings.Builder{}
	_, _ = fmt.Fprintf(&keyBuilder, "%d:%s:", groupID, strings.ToUpper(strings.TrimSpace(tier)))
	for _, id := range accountIDs {
		_, _ = fmt.Fprintf(&keyBuilder, "%d,", id)
	}
	counterValue, _ := openAIImageBillingRoutingCounters.LoadOrStore(keyBuilder.String(), &atomic.Uint64{})
	counter := counterValue.(*atomic.Uint64)
	start := int((counter.Add(1) - 1) % uint64(len(accountIDs)))
	rotated := make([]int64, 0, len(accountIDs))
	rotated = append(rotated, accountIDs[start:]...)
	rotated = append(rotated, accountIDs[:start]...)
	return rotated
}

func (s *OpenAIGatewayService) selectForcedOpenAIImageBillingAccount(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, bool, error) {
	accountIDs, tier, configured := s.imageBillingForcedAccountIDs(ctx, req.GroupID)
	if !configured {
		return nil, false, nil
	}

	platform := normalizeOpenAICompatiblePlatform(req.Platform)
	var waitAccount *Account
	reasons := make([]string, 0, len(accountIDs))

	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
		}
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[accountID]; excluded {
				reasons = append(reasons, fmt.Sprintf("excluded=%d", accountID))
				continue
			}
		}

		account, err := s.getSchedulableAccount(ctx, accountID)
		if err != nil || account == nil {
			reasons = append(reasons, fmt.Sprintf("not_schedulable=%d", accountID))
			continue
		}
		if account.Platform != platform || !account.IsOpenAICompatible() || !account.IsSchedulable() {
			reasons = append(reasons, fmt.Sprintf("platform_or_status_mismatch=%d", accountID))
			continue
		}
		if !s.openAIAccountMatchesSchedulingGroup(account, req.GroupID) {
			reasons = append(reasons, fmt.Sprintf("not_in_group=%d", accountID))
			continue
		}
		if shouldClearStickySession(account, req.RequestedModel) ||
			!isOpenAICompatibleAccountEligibleForRequest(ctx, account, platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability) ||
			!accountSupportsOpenAICapabilities(account, req.RequiredCapability, req.RequiredImageCapability) ||
			!s.isOpenAIAccountTransportCompatible(account, req.RequiredTransport) ||
			s.isOpenAIAccountRequestRuntimeBlocked(account, req.RequestedModel) ||
			!parentHealthyForShadow(account, s.parentAccountLookup(ctx)) {
			reasons = append(reasons, fmt.Sprintf("incompatible=%d", accountID))
			continue
		}
		if req.GroupID != nil && s.needsUpstreamChannelRestrictionCheck(ctx, req.GroupID) &&
			s.isUpstreamModelRestrictedByChannel(ctx, *req.GroupID, account, req.RequestedModel, req.RequireCompact) {
			reasons = append(reasons, fmt.Sprintf("channel_restricted=%d", accountID))
			continue
		}

		account = s.recheckSelectedOpenAIAccountFromDB(ctx, account, req.GroupID, platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
		if account == nil ||
			!s.openAIAccountMatchesSchedulingGroup(account, req.GroupID) ||
			!accountSupportsOpenAICapabilities(account, req.RequiredCapability, req.RequiredImageCapability) ||
			!s.isOpenAIAccountTransportCompatible(account, req.RequiredTransport) ||
			!parentHealthyForShadow(account, s.parentAccountLookup(ctx)) {
			reasons = append(reasons, fmt.Sprintf("recheck_failed=%d", accountID))
			continue
		}

		if result, acquireErr := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency); acquireErr == nil && result != nil && result.Acquired {
			slog.Debug("openai image billing forced account selected",
				"group_id", derefGroupID(req.GroupID),
				"tier", tier,
				"account_id", account.ID,
				"rotated_account_ids", accountIDs,
				"acquired", true,
			)
			selection, selectErr := s.newAcquiredSelectionResult(ctx, account, result.ReleaseFunc)
			return selection, true, selectErr
		}

		if waitAccount == nil {
			waitAccount = account
		}
	}

	if waitAccount != nil {
		cfg := s.schedulingConfig()
		slog.Debug("openai image billing forced account selected with wait plan",
			"group_id", derefGroupID(req.GroupID),
			"tier", tier,
			"account_id", waitAccount.ID,
			"rotated_account_ids", accountIDs,
		)
		selection, selectErr := s.newSelectionResult(ctx, waitAccount, false, nil, &AccountWaitPlan{
			AccountID:      waitAccount.ID,
			MaxConcurrency: waitAccount.Concurrency,
			Timeout:        cfg.FallbackWaitTimeout,
			MaxWaiting:     cfg.FallbackMaxWaiting,
		})
		return selection, true, selectErr
	}

	return nil, true, noAvailableOpenAISelectionError(req.RequestedModel, req.RequireCompact, fmt.Sprintf("image_billing_forced_%s_accounts_unavailable=%v reasons=%v", tier, accountIDs, reasons))
}
