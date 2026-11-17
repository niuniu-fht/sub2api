package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

func TestUsageEndToEndDurationMs(t *testing.T) {
	fallback := 1200 * time.Millisecond

	t.Run("no request start falls back to forward duration", func(t *testing.T) {
		got := usageEndToEndDurationMs(context.Background(), fallback)
		if got != 1200 {
			t.Fatalf("want fallback 1200, got %d", got)
		}
	})

	t.Run("zero request start falls back", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxkey.RequestStart, time.Time{})
		if got := usageEndToEndDurationMs(ctx, fallback); got != 1200 {
			t.Fatalf("want fallback 1200, got %d", got)
		}
	})

	t.Run("end-to-end includes retry and failover time", func(t *testing.T) {
		// 模拟请求 5 秒前进入（含两次失败换号 + 最终成功），最后一次转发仅 1.2s。
		start := time.Now().Add(-5 * time.Second)
		ctx := context.WithValue(context.Background(), ctxkey.RequestStart, start)
		got := usageEndToEndDurationMs(ctx, fallback)
		if got < 4900 || got > 5100 {
			t.Fatalf("want end-to-end ~5000ms, got %d", got)
		}
	})

	t.Run("end-to-end below forward duration keeps forward duration", func(t *testing.T) {
		// 异常场景：起点注入晚于 Forward 计时（理论上不该发生）。
		start := time.Now().Add(-100 * time.Millisecond)
		ctx := context.WithValue(context.Background(), ctxkey.RequestStart, start)
		if got := usageEndToEndDurationMs(ctx, fallback); got != 1200 {
			t.Fatalf("want fallback 1200, got %d", got)
		}
	})

	t.Run("async task wait beyond cap falls back", func(t *testing.T) {
		// Grok 视频类异步任务：提交后 20 分钟才计量，端到端超上限须回退单次口径。
		start := time.Now().Add(-20 * time.Minute)
		ctx := context.WithValue(context.Background(), ctxkey.RequestStart, start)
		if got := usageEndToEndDurationMs(ctx, fallback); got != 1200 {
			t.Fatalf("want fallback 1200, got %d", got)
		}
	})

	t.Run("detached context keeps request start value", func(t *testing.T) {
		// 异步计费链路（WithoutCancel 派生）仍应读到起点。
		start := time.Now().Add(-3 * time.Second)
		ctx := context.WithValue(context.Background(), ctxkey.RequestStart, start)
		detached := context.WithoutCancel(ctx)
		got := usageEndToEndDurationMs(detached, fallback)
		if got < 2900 || got > 3100 {
			t.Fatalf("want ~3000ms, got %d", got)
		}
	})
}

func TestResetOpsRequestStartForTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	oldStart := time.Now().Add(-8 * time.Second)
	ResetOpsRequestStartForTurn(c, oldStart)
	if _, ok := c.Request.Context().Value(ctxkey.RequestStart).(time.Time); !ok {
		t.Fatal("request start not injected into request context")
	}

	// 新一轮 turn 重置后，端到端口径以本轮为准（不再是 8 秒前）。
	turnStart := time.Now().Add(-500 * time.Millisecond)
	ResetOpsRequestStartForTurn(c, turnStart)
	start, ok := c.Request.Context().Value(ctxkey.RequestStart).(time.Time)
	if !ok || !start.Equal(turnStart) {
		t.Fatalf("turn start not reset: %v", start)
	}

	got := usageEndToEndDurationMs(c.Request.Context(), 100*time.Millisecond)
	if got < 400 || got > 600 {
		t.Fatalf("want turn-scoped ~500ms, got %d", got)
	}
}
