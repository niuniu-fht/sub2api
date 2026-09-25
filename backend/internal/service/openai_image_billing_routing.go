package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
)

const openAIAccountScheduleLayerImageBillingRouting = "image_billing_routing"

// imageBillingRoundRobinMaxSwitches 轮询链的账号切换上限(不含首次选中的账号)。
const imageBillingRoundRobinMaxSwitches = 3

func (s *OpenAIGatewayService) imageBillingForcedAccountIDs(ctx context.Context, groupID *int64) (accountIDs []int64, tier string, quality string, mode string, ok bool) {
	if s == nil || s.settingService == nil || groupID == nil || *groupID <= 0 {
		return nil, "", "", "", false
	}
	tier = ImageBillingSchedulingTierFromContext(ctx)
	if tier == "" {
		return nil, "", "", "", false
	}
	quality = ImageBillingSchedulingQualityFromContext(ctx)
	imageCount := ImageBillingSchedulingImageCountFromContext(ctx)
	settings := s.settingService.GetImageBillingAccountRoutingSettingsCached(ctx)
	accountIDs, mode = settings.AccountIDsAndModeFor(*groupID, quality, tier, imageCount)
	return accountIDs, tier, quality, mode, len(accountIDs) > 0
}

func (s *OpenAIGatewayService) selectForcedOpenAIImageBillingAccount(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, bool, error) {
	accountIDs, tier, quality, mode, configured := s.imageBillingForcedAccountIDs(ctx, req.GroupID)
	if !configured {
		return nil, false, nil
	}
	accountIDs = orderedOpenAIImageBillingAccountIDs(accountIDs, mode, req.SessionHash)

	platform := NormalizeOpenAICompatiblePlatform(req.Platform)
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
				"quality", quality,
				"mode", mode,
				"account_id", account.ID,
				"configured_account_ids", accountIDs,
				"acquired", true,
			)
			selection, selectErr := s.newAcquiredSelectionResult(ctx, account, result.ReleaseFunc)
			if selection != nil && mode == ImageBillingRoutingModeRoundRobin {
				// 轮询链:任意上游错误(含 400/连接错误)都切换下一个账号,最多 3 次。
				selection.ImageBillingRoundRobin = true
				selection.SwitchLimit = imageBillingRoundRobinMaxSwitches
			}
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
			"quality", quality,
			"mode", mode,
			"account_id", waitAccount.ID,
			"configured_account_ids", accountIDs,
		)
		selection, selectErr := s.newSelectionResult(ctx, waitAccount, false, nil, &AccountWaitPlan{
			AccountID:      waitAccount.ID,
			MaxConcurrency: waitAccount.Concurrency,
			Timeout:        cfg.FallbackWaitTimeout,
			MaxWaiting:     cfg.FallbackMaxWaiting,
		})
		return selection, true, selectErr
	}

	return nil, true, noAvailableOpenAISelectionError(req.RequestedModel, req.RequireCompact, fmt.Sprintf("image_billing_forced_%s_%s_accounts_unavailable=%v reasons=%v", quality, tier, accountIDs, reasons))
}

// imageBillingRoundRobinCounter 全局轮询计数器:每个请求递增,按取模结果
// 轮转账号链起点,保证请求在链上逐个轮转,而不是按会话哈希固定起点。
var imageBillingRoundRobinCounter atomic.Uint64

// orderedOpenAIImageBillingAccountIDs 按调度方式整理账号链顺序。
// priority 模式保持配置顺序;round_robin 模式按全局计数器逐请求轮转起点
// (会话哈希不再参与:单一大会话长期占用同一账号会使其余账号闲置)。
func orderedOpenAIImageBillingAccountIDs(ids []int64, mode string, sessionHash string) []int64 {
	if len(ids) < 2 || normalizeImageBillingRoutingMode(mode) != ImageBillingRoutingModeRoundRobin {
		return ids
	}
	offset := int((imageBillingRoundRobinCounter.Add(1) - 1) % uint64(len(ids)))
	out := make([]int64, 0, len(ids))
	out = append(out, ids[offset:]...)
	out = append(out, ids[:offset]...)
	return out
}
