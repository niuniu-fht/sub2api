package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeChannelStatusConfigDefaults(t *testing.T) {
	cfg, err := NormalizeChannelStatusConfig(ChannelStatusConfig{})
	require.NoError(t, err)
	require.Equal(t, 24, cfg.WindowHours)
	require.Equal(t, 60, cfg.RefreshSeconds)
	require.Equal(t, 99.0, cfg.Thresholds.Healthy)
	require.Equal(t, 90.0, cfg.Thresholds.Degraded)
	require.Empty(t, cfg.FilterRules)
}

func TestNormalizeChannelStatusConfigRejectsInvalid(t *testing.T) {
	base := DefaultChannelStatusConfig()

	// 窗口越界
	_, err := NormalizeChannelStatusConfig(ChannelStatusConfig{WindowHours: 24 * 31})
	require.Error(t, err)
	_, err = NormalizeChannelStatusConfig(ChannelStatusConfig{WindowHours: 0})
	require.NoError(t, err, "零值回落默认")

	// 阈值倒挂
	bad := base
	bad.Thresholds = ChannelStatusThresholds{Healthy: 80, Degraded: 90}
	_, err = NormalizeChannelStatusConfig(bad)
	require.Error(t, err)

	// 非法状态码
	bad = base
	bad.FilterRules = []ChannelStatusFilterRule{{StatusCodes: []int{99}}}
	_, err = NormalizeChannelStatusConfig(bad)
	require.Error(t, err)

	// 空规则被跳过
	bad = base
	bad.FilterRules = []ChannelStatusFilterRule{{}, {StatusCodes: []int{400}}}
	normalized, err := NormalizeChannelStatusConfig(bad)
	require.NoError(t, err)
	require.Len(t, normalized.FilterRules, 1)
}

func TestParseChannelStatusConfigFromSettingsJSON(t *testing.T) {
	cfg := ParseChannelStatusConfig(`{"window_hours":1,"refresh_seconds":30,
		"filter_rules":[{"status_codes":[400],"keywords":["content_policy"]}],
		"thresholds":{"healthy":98,"degraded":85}}`)
	require.Equal(t, 1, cfg.WindowHours)
	require.Equal(t, 30, cfg.RefreshSeconds)
	require.Len(t, cfg.FilterRules, 1)
	require.Equal(t, 98.0, cfg.Thresholds.Healthy)
	require.Equal(t, 85.0, cfg.Thresholds.Degraded)

	// 非法 JSON 回落默认
	cfg = ParseChannelStatusConfig("not-json")
	require.Equal(t, DefaultChannelStatusConfig(), cfg)
}

func TestMatchesChannelStatusFilterRules(t *testing.T) {
	rules := []ChannelStatusFilterRule{
		{StatusCodes: []int{400, 429}},
		{Keywords: []string{"content_policy", "quota exceeded"}},
		{StatusCodes: []int{500}, Keywords: []string{"internal"}},
	}

	type testCase struct {
		name       string
		upstream   int
		client     int
		message    string
		wantFilter bool
	}
	cases := []testCase{
		{"status only match", 400, 400, "invalid argument", true},
		{"status no match", 500, 500, "server error", false},
		{"keyword match", 400, 400, "blocked by CONTENT_POLICY", true},
		{"keyword match other rule", 500, 500, "you have QUOTA EXCEEDED already", true},
		{"no match at all", 403, 403, "forbidden", false},
		// 第三条规则：状态码+关键词需同时命中
		{"and rule both hit", 500, 500, "internal error", true},
		{"and rule status miss", 403, 403, "internal error", false},
		{"and rule keyword miss", 500, 500, "custom failure", false},
		// 上游状态码为 0（未到达上游）时回退客户端状态码
		{"client fallback", 0, 429, "rate limited", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantFilter,
				MatchesChannelStatusFilterRules(rules, tc.upstream, tc.client, tc.message))
		})
	}

	// 空规则集不过滤
	require.False(t, MatchesChannelStatusFilterRules(nil, 400, 400, "anything"))
}
