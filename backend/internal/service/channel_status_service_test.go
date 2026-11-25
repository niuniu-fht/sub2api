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

// 一次请求 A 错 B 错 C 成功：客户端最终 200，ops 行以 "Recovered …" 记录
// 中间失败。分组/总体（用户视角）必须把这些行排除在失败之外；账号层面
// （尝试视角）保留并按真实上游码/传输层分类。样例消息均取自生产真实数据。
func TestClassifyStatus200_ProductionShapes(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    string
		skip    bool
	}{
		{"上游码恢复", "Recovered upstream error 504: <badge color=\"success\">图片生成成功</badge>", "↻504", false},
		{"上游码恢复401", "Recovered upstream error 401: Invalid token (request id: 2026110417)", "↻401", false},
		{"传输层恢复", "Recovered upstream error: Post \"http://leoapp:8099/v1/images/generations\": dial tcp: lookup leoapp: no such host", "↻net", false},
		{"账号鉴权恢复", "Recovered account authentication failure", "↻net", false},
		{"内嵌真实码", "上游返回 HTTP 429，请稍后重试", "429", false},
		{"业务失败保持200", "Accounts are temporarily unavailable. Please retry later.", "200", false},
		{"裸成功文案跳过", "<badge color=\"success\">图片生成成功</badge>", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, skip := classifyStatus200(tc.message)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.skip, skip)
		})
	}
}

// 分组口径（用户视角）：所有 "Recovered …" 形态的 200 行都跳过——
// 包括无状态码的传输层恢复，它们同样是「换号后最终成功」的请求。
func TestIsRecoveredStatus200Row_AllRecoveredForms(t *testing.T) {
	require.True(t, isRecoveredStatus200Row("Recovered upstream error 502: boom"))
	require.True(t, isRecoveredStatus200Row("Recovered upstream error: dial tcp: i/o timeout"))
	require.True(t, isRecoveredStatus200Row("Recovered account authentication failure"))
	require.False(t, isRecoveredStatus200Row("Accounts are temporarily unavailable. Please retry later."))
	require.False(t, isRecoveredStatus200Row(""))
}
