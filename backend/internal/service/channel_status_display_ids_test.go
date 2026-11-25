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

// 未配置白名单：输出全部有记录的公开分组，专属分组不进快照。
func TestChannelStatusDisplayIDs_NoWhitelist(t *testing.T) {
	got := channelStatusDisplayIDs([]int64{10, 20, 40}, nil, channelStatusTestGroupMeta())
	require.Equal(t, []int64{10, 20}, got)
}

// 配置白名单：只输出白名单内有记录的分组，保持有记录分组的相对顺序。
func TestChannelStatusDisplayIDs_WhitelistFilters(t *testing.T) {
	got := channelStatusDisplayIDs([]int64{10, 20, 30}, []int64{30, 10}, channelStatusTestGroupMeta())
	require.Equal(t, []int64{10, 30}, got)
}

// 白名单里窗口内没有记录的分组也要输出（追加在末尾），否则管理员勾选后看不到。
func TestChannelStatusDisplayIDs_WhitelistBackfillsIdleGroups(t *testing.T) {
	got := channelStatusDisplayIDs([]int64{20}, []int64{20, 30}, channelStatusTestGroupMeta())
	require.Equal(t, []int64{20, 30}, got)
}

// 专属分组与已删除分组即使出现在白名单里也绝不输出。
func TestChannelStatusDisplayIDs_NeverLeaksExclusiveOrDeleted(t *testing.T) {
	got := channelStatusDisplayIDs([]int64{20, 40}, []int64{40, 99, 20}, channelStatusTestGroupMeta())
	require.Equal(t, []int64{20}, got)
}

// 白名单重复项只输出一次。
func TestChannelStatusDisplayIDs_DeduplicatesWhitelist(t *testing.T) {
	got := channelStatusDisplayIDs(nil, []int64{10, 10, 30}, channelStatusTestGroupMeta())
	require.Equal(t, []int64{10, 30}, got)
}
