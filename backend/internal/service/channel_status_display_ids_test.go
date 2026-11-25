//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func channelStatusTestGroupMeta() map[int64]channelStatusGroupMeta {
	return map[int64]channelStatusGroupMeta{
		10: {Name: "g10", Platform: "openai"},
		20: {Name: "g20", Platform: "openai"},
		30: {Name: "g30", Platform: "openai"},
		40: {Name: "g40", Platform: "gemini", Exclusive: true},
	}
}

// 未配置白名单：输出全部有记录的公开分组（保持传入顺序），专属分组不进快照。
func TestChannelStatusDisplayIDs_NoWhitelist(t *testing.T) {
	got := channelStatusDisplayIDs([]int64{40, 20, 10}, nil, channelStatusTestGroupMeta())
	require.Equal(t, []int64{20, 10}, got)
}

// 配置白名单：输出顺序即白名单顺序（管理员定义的状态页展示顺序），与 ID 大小、流量无关。
func TestChannelStatusDisplayIDs_WhitelistOrderIsDisplayOrder(t *testing.T) {
	got := channelStatusDisplayIDs([]int64{10, 20, 30}, []int64{30, 10, 20}, channelStatusTestGroupMeta())
	require.Equal(t, []int64{30, 10, 20}, got)
}

// 白名单里窗口内没有记录的分组也按其在白名单中的位置输出，
// 否则管理员勾选后看不到，会被误认为配置未生效。
func TestChannelStatusDisplayIDs_WhitelistKeepsGroupsWithoutRecords(t *testing.T) {
	got := channelStatusDisplayIDs([]int64{20, 10}, []int64{30, 20}, channelStatusTestGroupMeta())
	require.Equal(t, []int64{30, 20}, got)
}

// 专属分组与已删除分组即使出现在白名单里也绝不输出。
func TestChannelStatusDisplayIDs_NeverLeaksExclusiveOrDeleted(t *testing.T) {
	got := channelStatusDisplayIDs([]int64{20, 40}, []int64{40, 99, 20}, channelStatusTestGroupMeta())
	require.Equal(t, []int64{20}, got)
}

// 白名单重复项只输出一次（保留首个位置）。
func TestChannelStatusDisplayIDs_DeduplicatesWhitelist(t *testing.T) {
	got := channelStatusDisplayIDs(nil, []int64{10, 10, 30}, channelStatusTestGroupMeta())
	require.Equal(t, []int64{10, 30}, got)
}
