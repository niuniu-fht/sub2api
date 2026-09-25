package service

import "testing"

func TestAccountIDsAndModeForUnmatchedFallback(t *testing.T) {
	settings := ImageBillingAccountRoutingSettings{Groups: map[int64]ImageBillingGroupAccountRouting{
		20: {
			Rules: []OpenAIImageBillingRoutingRule{
				{Quality: "low", Tier: "1K", Mode: "round_robin", AccountIDs: []int64{11, 12}},
				{Quality: ImageQualityUnmatched, Tier: "1K", Mode: "round_robin", AccountIDs: []int64{21, 22}},
				{Quality: ImageQualityUnmatched, Tier: "2K", AccountIDs: []int64{23}},
			},
			// 旧档位兜底字段(历史数据)
			FourKAccountIDs: []int64{31},
		},
	}}

	cases := []struct {
		name       string
		quality    string
		tier       string
		imageCount int
		wantIDs    []int64
	}{
		{"low 命中 low 规则", "low", "1K", 0, []int64{11, 12}},
		{"不带 quality 走不匹配", "", "1K", 0, []int64{21, 22}},
		{"auto 走不匹配", "auto", "1K", 0, []int64{21, 22}},
		{"未配置的 high 走不匹配", "high", "1K", 0, []int64{21, 22}},
		{"2K 无 low 规则走不匹配", "low", "2K", 0, []int64{23}},
		{"4K 无规则走旧档位兜底字段", "", "4K", 0, []int64{31}},
	}
	for _, tc := range cases {
		ids, mode := settings.AccountIDsAndModeFor(20, tc.quality, tc.tier, tc.imageCount)
		if len(ids) != len(tc.wantIDs) {
			t.Fatalf("%s: ids=%v, want %v", tc.name, ids, tc.wantIDs)
		}
		for i := range ids {
			if ids[i] != tc.wantIDs[i] {
				t.Fatalf("%s: ids=%v, want %v", tc.name, ids, tc.wantIDs)
			}
		}
		if tc.name == "low 命中 low 规则" && mode != ImageBillingRoutingModeRoundRobin {
			t.Fatalf("low rule mode=%q, want round_robin", mode)
		}
	}
}

func TestNormalizeImageBillingRuleQualityAcceptsUnmatched(t *testing.T) {
	if got := normalizeImageBillingRuleQuality("Unmatched"); got != ImageQualityUnmatched {
		t.Fatalf("got %q", got)
	}
	if got := normalizeImageBillingRuleQuality("HIGH"); got != "high" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeImageBillingRuleQuality("unknown-x"); got != "" {
		t.Fatalf("unknown quality should be dropped, got %q", got)
	}
}
