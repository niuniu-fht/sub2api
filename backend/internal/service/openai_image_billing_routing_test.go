package service

import (
	"testing"

)

func firstIDs(ids []int64, mode, sessionHash string, n int) []int64 {
	firsts := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		ordered := orderedOpenAIImageBillingAccountIDs(ids, mode, sessionHash)
		firsts = append(firsts, ordered[0])
	}
	return firsts
}

func TestOrderedOpenAIImageBillingRoundRobinRotatesPerRequest(t *testing.T) {
	ids := []int64{101, 102, 103, 104}
	firsts := firstIDs(ids, "round_robin", "session-a", len(ids))
	for i, got := range firsts {
		want := ids[i%len(ids)]
		if got != want {
			t.Fatalf("call %d first=%d, want %d (consecutive rotation)", i, got, want)
		}
	}
}

func TestOrderedOpenAIImageBillingRoundRobinIgnoresSession(t *testing.T) {
	ids := []int64{101, 102, 103}
	// 同一会话的连续调用也必须轮转,不再按会话哈希固定起点
	seen := map[int64]bool{}
	for _, got := range firstIDs(ids, "round_robin", "session-a", len(ids)+1) {
		seen[got] = true
	}
	if len(seen) != len(ids) {
		t.Fatalf("same session must rotate across all accounts, seen=%v", seen)
	}
}

func TestOrderedOpenAIImageBillingPriorityKeepsOrder(t *testing.T) {
	ids := []int64{101, 102, 103}
	firsts := firstIDs(ids, "priority", "session-a", 5)
	for i, got := range firsts {
		if got != ids[0] {
			t.Fatalf("priority call %d first=%d, want %d", i, got, ids[0])
		}
	}
}

func TestOrderedOpenAIImageBillingShortChainUnchanged(t *testing.T) {
	ids := []int64{101}
	got := orderedOpenAIImageBillingAccountIDs(ids, "round_robin", "")
	if len(got) != 1 || got[0] != 101 {
		t.Fatalf("single account chain must be unchanged, got %v", got)
	}
}

func TestNormalizeImageBillingRuleFallbackFields(t *testing.T) {
	rules := []OpenAIImageBillingRoutingRule{
		// 合法:兜底账号在列表内,次数夹取
		{Quality: "high", Tier: "2K", AccountIDs: []int64{11, 12, 13}, FallbackAccountID: 12, FallbackRetryCount: 99},
		// 非法:兜底账号不在列表内 → 清零
		{Quality: "high", Tier: "4K", AccountIDs: []int64{21, 22}, FallbackAccountID: 999, FallbackRetryCount: 5},
		// 未配置 → 保持零值
		{Quality: "low", Tier: "1K", AccountIDs: []int64{31, 32}},
	}
	got := normalizeOpenAIImageBillingRoutingRules(rules)
	if len(got) != 3 {
		t.Fatalf("rules=%d, want 3", len(got))
	}
	if got[0].FallbackAccountID != 12 || got[0].FallbackRetryCount != 10 {
		t.Fatalf("rule0 fallback=%d/%d, want 12/10 (clamped)", got[0].FallbackAccountID, got[0].FallbackRetryCount)
	}
	if got[1].FallbackAccountID != 0 || got[1].FallbackRetryCount != 0 {
		t.Fatalf("rule1 fallback must be cleared, got %d/%d", got[1].FallbackAccountID, got[1].FallbackRetryCount)
	}
	if got[2].FallbackAccountID != 0 || got[2].FallbackRetryCount != 0 {
		t.Fatalf("rule2 fallback must stay zero, got %d/%d", got[2].FallbackAccountID, got[2].FallbackRetryCount)
	}
}

func TestImageBillingFallbackRetryFor(t *testing.T) {
	settings := ImageBillingAccountRoutingSettings{Groups: map[int64]ImageBillingGroupAccountRouting{
		40: {Rules: []OpenAIImageBillingRoutingRule{
			{Quality: "unmatched", Tier: "2K", Mode: "priority", AccountIDs: []int64{256, 267, 278}, FallbackAccountID: 267, FallbackRetryCount: 5},
			{Quality: "unmatched", Tier: "4K", Mode: "priority", AccountIDs: []int64{256, 267}, FallbackRetryCount: 0},
		}},
	}}

	fbID, fbN := settings.FallbackRetryFor(40, "max", "2K", 0)
	if fbID != 267 || fbN != 5 {
		t.Fatalf("exact quality: fallback=%d/%d, want 267/5", fbID, fbN)
	}
	// quality 未命中任何具体规则 → 落到 unmatched 兜底规则
	if fbID, fbN = settings.FallbackRetryFor(40, "max", "2K", 2); fbID != 267 || fbN != 5 {
		// max 命中 unmatched(无 max 专属规则)——同上应返回兜底配置
		t.Fatalf("unmatched quality: fallback=%d/%d, want 267/5", fbID, fbN)
	}
	if fbID, fbN = settings.FallbackRetryFor(40, "max", "4K", 0); fbID != 0 || fbN != 0 {
		t.Fatalf("4K rule has no fallback, got %d/%d", fbID, fbN)
	}
	if fbID, fbN = settings.FallbackRetryFor(999, "high", "2K", 0); fbID != 0 || fbN != 0 {
		t.Fatalf("unknown group must return zero, got %d/%d", fbID, fbN)
	}
}
