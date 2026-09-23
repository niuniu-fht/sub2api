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
