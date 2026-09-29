package service

import "testing"

func TestImageCountZeroRouting(t *testing.T) {
	settings := ImageBillingAccountRoutingSettings{Groups: map[int64]ImageBillingGroupAccountRouting{
		20: {Rules: []OpenAIImageBillingRoutingRule{
			// 0 张专属(文生图)
			{Quality: "unmatched", Tier: "1K", ImageCounts: []int{0}, AccountIDs: []int64{101}},
			// 1 张专属
			{Quality: "unmatched", Tier: "1K", ImageCounts: []int{1}, AccountIDs: []int64{102}},
			// 不限
			{Quality: "unmatched", Tier: "1K", AccountIDs: []int64{103}},
		}},
	}}

	cases := []struct {
		name    string
		count   int
		wantTop int64
	}{
		{"文生图(0张)命中0专属规则", 0, 101},
		{"1张参考图命中1专属规则", 1, 102},
		{"2张参考图回退不限规则", 2, 103},
	}
	for _, tc := range cases {
		ids, _ := settings.AccountIDsAndModeFor(20, "", "1K", tc.count)
		if len(ids) == 0 || ids[0] != tc.wantTop {
			t.Fatalf("%s: got %v, want first=%d", tc.name, ids, tc.wantTop)
		}
	}
}

func TestNormalizeImageBillingRuleImageCountsAllowsZero(t *testing.T) {
	got := normalizeImageBillingRuleImageCounts([]int{0, 1, 5, 5, -1, 33})
	if len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 5 {
		t.Fatalf("got %v, want [0 1 5]", got)
	}
}
