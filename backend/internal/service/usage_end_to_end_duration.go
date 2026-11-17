package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

// maxUsageEndToEndDurationMs 是端到端口径的采用上限：
// 异步计费链路（如 Grok 视频任务提交后数分钟才轮询出结果再计量）从提交到
// 计量的间隔远超请求本身耗时，直接采用会把几分钟的任务等待写成请求耗时。
// 端到端值超过该上限时回退最后一次转发的单次口径。
const maxUsageEndToEndDurationMs = 10 * 60 * 1000

// usageEndToEndDurationMs 返回 usage 记录应落库的耗时（毫秒）。
//
// 口径：优先取「请求进入网关 → 本次计量提交」的端到端耗时——middleware 在请求
// 最外层把起点注入 request context（ctxkey.RequestStart），因此账号切换、失败
// 重试、选号等待的整段时间都计入 duration_ms。fallback 是最后一次成功转发的
// 耗时（Forward 内部计时），在以下情况回退：
//   - ctx 里没有起点（不走全局 middleware 的调用方，如部分测试/任务路径）；
//   - 起点为零值或推算出的耗时非正（异常时钟）；
//   - 端到端值超过 maxUsageEndToEndDurationMs（异步任务等待，见上）。
//
// 端到端值小于 fallback（中间件注入晚于 Forward 计时等异常）时同样采用
// fallback，保证 duration 永远不小于真实的最后一次转发耗时。
//
// 入站 WebSocket 每轮 turn 开始时会重置起点（BeginOpsStreamTurn），因此多轮
// 连接上每轮 usage 的耗时仍是该轮的端到端口径。
func usageEndToEndDurationMs(ctx context.Context, fallback time.Duration) int {
	fallbackMs := int(fallback.Milliseconds())
	if ctx != nil {
		if start, ok := ctx.Value(ctxkey.RequestStart).(time.Time); ok && !start.IsZero() {
			if elapsedMs := int(time.Since(start).Milliseconds()); elapsedMs > fallbackMs && elapsedMs <= maxUsageEndToEndDurationMs {
				return elapsedMs
			}
		}
	}
	return fallbackMs
}

// ResetOpsRequestStartForTurn 将请求起点重置为当前时刻。
// 用于入站 WebSocket 的新一轮 turn：usage 的端到端口径以本轮为准，
// 而不是把连接建立以来的空闲时间计入。
func ResetOpsRequestStartForTurn(c *gin.Context, now time.Time) {
	if c == nil || c.Request == nil {
		return
	}
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.RequestStart, now))
}
