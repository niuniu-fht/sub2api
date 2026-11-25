//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// channelStatusSettingRepoStub 是聚合读取开关/配置所需的设置仓库桩（只实现 GetMultiple）。
type channelStatusSettingRepoStub struct {
	SettingRepository
	values map[string]string
}

func (s channelStatusSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return s.values, nil
}

func newChannelStatusAggregateTestService(t *testing.T, displayGroupIDs []int64) (*ChannelStatusService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	config, err := json.Marshal(ChannelStatusConfig{
		WindowHours:     24,
		RefreshSeconds:  60,
		Thresholds:      ChannelStatusThresholds{Healthy: 90, Degraded: 70},
		DisplayGroupIDs: displayGroupIDs,
	})
	require.NoError(t, err)
	settings := NewSettingService(channelStatusSettingRepoStub{values: map[string]string{
		SettingKeyChannelStatusEnabled: "true",
		SettingKeyChannelStatusConfig:  string(config),
	}}, nil)
	return NewChannelStatusService(settings, db, nil), mock
}

// 整条聚合链路（sqlmock 代替数据库）覆盖「展示白名单里勾选了窗口内没有记录的分组」：
// 该分组必须以 unknown 输出且带 24 个空桶，专属分组即使被勾选也不输出。
// 回归：无记录分组的桶切片尚未分配，旧实现组装时间线时越界 panic，并因发生在保存配置后的
// 即时聚合 goroutine 里而终止了整个进程。
func TestChannelStatusAggregate_WhitelistedGroupWithoutRecords(t *testing.T) {
	svc, mock := newChannelStatusAggregateTestService(t, []int64{20, 60, 61})

	mock.ExpectQuery(`AS bucket_idx\s+FROM usage_logs`).WillReturnRows(
		sqlmock.NewRows([]string{"group_id", "model", "requests", "avg_ms", "bucket_idx"}).
			AddRow(20, "gpt-image-2", 10, 40000.0, 0).
			AddRow(20, "gpt-image-2", 5, 50000.0, 1),
	)
	mock.ExpectQuery(`count\(\*\), avg\(duration_ms\)\s+FROM usage_logs`).WillReturnRows(
		sqlmock.NewRows([]string{"group_id", "count", "avg"}).AddRow(20, 3, 42000.0),
	)
	mock.ExpectQuery(`FROM ops_error_logs\s+WHERE created_at >= \$1 AND group_id IS NOT NULL`).WillReturnRows(
		sqlmock.NewRows([]string{"group_id", "model", "upstream_status_code", "status_code", "message", "created_at"}).
			AddRow(20, "gpt-image-2", 502, 502, "upstream boom", time.Now()),
	)
	mock.ExpectQuery(`GROUP BY GROUPING SETS`).WillReturnRows(
		sqlmock.NewRows([]string{"group_id", "model", "bucket_idx", "avg", "min", "max", "p95", "p99"}).
			AddRow(20, "gpt-image-2", nil, 42000.0, 30000.0, 60000.0, 55000.0, 60000.0),
	)
	mock.ExpectQuery(`FROM groups`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "name", "platform", "exclusive"}).
			AddRow(20, "低质量", "openai", false).
			AddRow(60, "高质量稳定", "openai", false).
			AddRow(61, "专属分组", "openai", true),
	)

	snapshot, err := svc.Aggregate(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, snapshot.Groups, 2, "专属分组 61 即使在白名单里也不输出")

	active := snapshot.Groups[0]
	require.Equal(t, int64(20), active.ID)
	require.Equal(t, "operational", active.Status)
	require.Equal(t, int64(15), active.Requests)
	require.Equal(t, int64(1), active.Failures)
	require.Len(t, active.Models, 1)
	require.NotNil(t, active.LastHour)
	require.Len(t, active.Timeline, channelStatusTimelineBuckets)
	require.Equal(t, int64(10), active.Timeline[0].Requests)
	require.Equal(t, int64(5), active.Timeline[1].Requests)

	idle := snapshot.Groups[1]
	require.Equal(t, int64(60), idle.ID)
	require.Equal(t, "高质量稳定", idle.Name)
	require.Equal(t, "unknown", idle.Status)
	require.Equal(t, float64(-1), idle.SuccessRate)
	require.Zero(t, idle.Requests)
	require.Zero(t, idle.Failures)
	require.Empty(t, idle.Models)
	require.Nil(t, idle.LastHour)
	require.Len(t, idle.Timeline, channelStatusTimelineBuckets)
	for i, bucket := range idle.Timeline {
		require.Zero(t, bucket.Requests, "bucket %d", i)
		require.Zero(t, bucket.Failures, "bucket %d", i)
		require.Equal(t, "unknown", bucket.Status, "bucket %d", i)
	}
}

// 统计聚合是旁路功能：内部 panic 必须收敛为 error 返回，绝不能穿透到调用它的 goroutine
// （定时刷新与「保存配置后立即聚合」都在独立 goroutine 中运行）。
func TestChannelStatusAggregate_ConvertsPanicToError(t *testing.T) {
	// 开关已开但没有数据库：首次查询空指针解引用，用于制造 panic。
	svc, _ := newChannelStatusAggregateTestService(t, nil)
	svc.db = nil

	snapshot, err := svc.Aggregate(context.Background())
	require.Nil(t, snapshot)
	require.ErrorContains(t, err, "channel status aggregation panicked")
}
