//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 成功侧 SQL 改为按「模型 × 小时桶」聚合后，每个小时的请求必须落进自己的桶；
// 旧实现用 max(created_at) 当整组时间戳，会把全窗口成功堆进最近一小时的桶，
// 历史小时桶只剩失败（成功率 0）。
func TestChannelStatusAgg_HourlyBucketAttribution(t *testing.T) {
	start := time.Date(2026, 11, 13, 0, 37, 0, 0, time.UTC) // 窗口起点任意分钟
	agg := newChannelStatusAgg(start, 24)
	require.Equal(t, float64(3600), agg.bucketSeconds, "24h 窗口 24 桶 → 每桶 1 小时")

	// 第 0 桶内两个模型；第 1、2 小时各一个模型。
	agg.addSuccess(11, "model-a", 7, 2000, start.Add(10*time.Minute))
	agg.addSuccess(11, "model-b", 3, 1000, start.Add(20*time.Minute))
	agg.addSuccess(11, "model-a", 5, 4000, start.Add(90*time.Minute))
	agg.addSuccess(11, "model-c", 2, 500, start.Add(2*time.Hour+5*time.Minute))

	require.Equal(t, int64(10), agg.bucketRequests[11][0]) // model-a 7 + model-b 3
	require.Equal(t, int64(5), agg.bucketRequests[11][1])
	require.Equal(t, int64(2), agg.bucketRequests[11][2])
	for i := 3; i < channelStatusTimelineBuckets; i++ {
		require.Zero(t, agg.bucketRequests[11][i], "bucket %d should be empty", i)
	}

	// 失败侧仍为行级时间戳落桶。
	agg.addFailure(11, "model-a", start.Add(40*time.Minute))
	agg.addFailure(11, "model-a", start.Add(2*time.Hour+10*time.Minute))
	require.Equal(t, int64(1), agg.bucketFailures[11][0])
	require.Equal(t, int64(1), agg.bucketFailures[11][2])
}

// 每桶的模型明细：成功率/平均耗时只统计本桶数据。
func TestChannelStatusAgg_BucketModelsDrilldown(t *testing.T) {
	start := time.Date(2026, 11, 13, 5, 0, 0, 0, time.UTC)
	agg := newChannelStatusAgg(start, 24)

	// 第 0 桶：model-a 成功 3 次（均耗 1000ms）失败 1 次 → 75%；model-b 成功 1 次 → 100%。
	agg.addSuccess(11, "model-a", 3, 1000, start.Add(30*time.Minute))
	agg.addFailure(11, "model-a", start.Add(35*time.Minute))
	agg.addSuccess(11, "model-b", 1, 6000, start.Add(40*time.Minute))
	// 第 1 桶：model-a 成功 1 次但失败 3 次 → 25%（旧实现这里恒为 0）。
	agg.addSuccess(11, "model-a", 1, 2000, start.Add(90*time.Minute))
	agg.addFailure(11, "model-a", start.Add(95*time.Minute))
	agg.addFailure(11, "model-a", start.Add(100*time.Minute))
	agg.addFailure(11, "model-a", start.Add(105*time.Minute))

	b0 := agg.bucketModels(11, 0)
	require.Len(t, b0, 2)
	require.Equal(t, "model-a", b0[0].Model) // 按请求量降序
	require.Equal(t, int64(3), b0[0].Requests)
	require.Equal(t, int64(1), b0[0].Failures)
	require.InDelta(t, 75.0, b0[0].SuccessRate, 1e-9)
	require.Equal(t, int64(1000), b0[0].AvgGenMs)
	require.Equal(t, "model-b", b0[1].Model)
	require.InDelta(t, 100.0, b0[1].SuccessRate, 1e-9)
	require.Equal(t, int64(6000), b0[1].AvgGenMs)

	b1 := agg.bucketModels(11, 1)
	require.Len(t, b1, 1)
	require.Equal(t, "model-a", b1[0].Model)
	require.InDelta(t, 25.0, b1[0].SuccessRate, 1e-9)
	require.Equal(t, int64(2000), b1[0].AvgGenMs)

	// 空桶无明细。
	require.Empty(t, agg.bucketModels(11, 5))
	// 越界索引安全返回。
	require.Nil(t, agg.bucketModels(11, -1))
	require.Nil(t, agg.bucketModels(11, channelStatusTimelineBuckets))
	require.Nil(t, agg.bucketModels(999, 0))
}
