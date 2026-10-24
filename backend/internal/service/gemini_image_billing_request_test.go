package service

import (
	"testing"
)

// TestParseGeminiImageBillingRequestInfoRefImages 验证参考图计数:
// camelCase/snake_case 内联图与 fileData 文件引用都计入,纯文本为 0。
func TestParseGeminiImageBillingRequestInfoRefImages(t *testing.T) {
	camel := []byte(`{
		"contents": [{"role": "user", "parts": [
			{"text": "edit this"},
			{"inlineData": {"mimeType": "image/png", "data": "aaa"}},
			{"inlineData": {"mimeType": "image/png", "data": "bbb"}}
		]}],
		"generationConfig": {"responseModalities": ["TEXT", "IMAGE"], "imageConfig": {"aspectRatio": "3:4", "imageSize": "2K"}}
	}`)
	info := ParseGeminiImageBillingRequestInfo(camel)
	if info.RefImageCount != 2 {
		t.Fatalf("camel RefImageCount = %d, want 2", info.RefImageCount)
	}
	if info.Tier != "2K" || info.AspectRatio != "3:4" {
		t.Fatalf("tier/aspect = %q/%q", info.Tier, info.AspectRatio)
	}

	snake := []byte(`{
		"contents": [{"role": "user", "parts": [
			{"inline_data": {"mime_type": "image/png", "data": "aaa"}},
			{"file_data": {"file_uri": "https://example.com/a.png"}}
		]}],
		"generationConfig": {"response_modalities": ["TEXT", "IMAGE"]}
	}`)
	info = ParseGeminiImageBillingRequestInfo(snake)
	if info.RefImageCount != 2 {
		t.Fatalf("snake RefImageCount = %d, want 2", info.RefImageCount)
	}
	if info.Tier != "1K" {
		t.Fatalf("default tier = %q, want 1K", info.Tier)
	}

	textOnly := []byte(`{"contents": [{"role": "user", "parts": [{"text": "a cat"}]}]}`)
	info = ParseGeminiImageBillingRequestInfo(textOnly)
	if info.RefImageCount != 0 {
		t.Fatalf("textOnly RefImageCount = %d, want 0", info.RefImageCount)
	}
}

// TestGeminiImageBillingSettingsAccountIDsForCounts 验证参考图数量路由匹配:
// 数量专用规则优先命中,数量不限的泛化规则兜底,数量不匹配则跳过。
func TestGeminiImageBillingSettingsAccountIDsForCounts(t *testing.T) {
	settings := GeminiImageBillingRoutingSettings{Groups: map[int64]GeminiImageBillingGroupRouting{
		24: {Rules: []GeminiImageBillingRoutingRule{
			{Tier: "1K", AspectRatio: "*", ImageCounts: []int{0, 1}, AccountIDs: []int64{111}},
			{Tier: "1K", AspectRatio: "*", ImageCounts: []int{2, 3, 4}, AccountIDs: []int64{222}},
		}},
	}}

	cases := []struct {
		count int
		want  []int64
	}{
		{0, []int64{111}},
		{1, []int64{111}},
		{2, []int64{222}},
		{4, []int64{222}},
	}
	for _, tc := range cases {
		got := settings.AccountIDsFor(24, "1K", "*", tc.count)
		if len(got) != 1 || got[0] != tc.want[0] {
			t.Fatalf("count=%d got %v, want %v", tc.count, got, tc.want)
		}
	}

	// 超出数量专用规则的范围时回落到泛化规则(数量不限)
	generic := GeminiImageBillingRoutingSettings{Groups: map[int64]GeminiImageBillingGroupRouting{
		24: {Rules: []GeminiImageBillingRoutingRule{
			{Tier: "1K", AspectRatio: "*", ImageCounts: []int{2, 3, 4}, AccountIDs: []int64{222}},
			{Tier: "1K", AspectRatio: "*", AccountIDs: []int64{333}},
		}},
	}}
	if got := generic.AccountIDsFor(24, "1K", "*", 1); len(got) != 1 || got[0] != 333 {
		t.Fatalf("generic fallback got %v, want [333]", got)
	}
	if got := generic.AccountIDsFor(24, "1K", "*", 3); len(got) != 1 || got[0] != 222 {
		t.Fatalf("count-specific priority got %v, want [222]", got)
	}
}

// TestGeminiImageBillingSettingsAccountAllowedForCounts 验证约束过滤的参考图语义:
// 账号被规则约束时,仅当档位/比例/参考图数量全部匹配才可用。
func TestGeminiImageBillingSettingsAccountAllowedForCounts(t *testing.T) {
	settings := GeminiImageBillingRoutingSettings{Groups: map[int64]GeminiImageBillingGroupRouting{
		24: {Rules: []GeminiImageBillingRoutingRule{
			{Tier: "1K", AspectRatio: "*", ImageCounts: []int{0, 1}, AccountIDs: []int64{111}},
		}},
	}}

	if !settings.AccountAllowedFor(24, 111, "1K", "*", 1) {
		t.Fatal("count=1 should be allowed")
	}
	if settings.AccountAllowedFor(24, 111, "1K", "*", 3) {
		t.Fatal("count=3 should be constrained away")
	}
	// 未被任何规则约束的账号不受影响
	if !settings.AccountAllowedFor(24, 999, "1K", "*", 3) {
		t.Fatal("unconstrained account should always be allowed")
	}
}
