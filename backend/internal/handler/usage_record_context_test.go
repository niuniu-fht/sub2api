package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// TestUsageRecordContextCarriesRequestStart 防回归：异步计费 worker 的 ctx 由
// usageRecordContext 从请求 ctx 重建，必须复制 RequestStart——丢失会静默回退
// "仅最后一次转发耗时"的旧口径（issue：多账号切换的失败段不计入 duration_ms）。
func TestUsageRecordContextCarriesRequestStart(t *testing.T) {
	start := time.Now().Add(-5 * time.Second)
	parent := context.WithValue(context.Background(), ctxkey.RequestStart, start)
	parent = context.WithValue(parent, ctxkey.RequestID, "req-1")

	got := usageRecordContext(parent, context.Background())

	v, ok := got.Value(ctxkey.RequestStart).(time.Time)
	if !ok {
		t.Fatal("RequestStart not copied into usage record context")
	}
	if !v.Equal(start) {
		t.Fatalf("RequestStart mismatch: want %v, got %v", start, v)
	}
	if id, _ := got.Value(ctxkey.RequestID).(string); id != "req-1" {
		t.Fatalf("RequestID lost: %q", id)
	}
}

func TestUsageRecordContextWithoutRequestStart(t *testing.T) {
	got := usageRecordContext(context.Background(), context.Background())
	if _, ok := got.Value(ctxkey.RequestStart).(time.Time); ok {
		t.Fatal("unexpected RequestStart in empty parent")
	}
}

func TestUsageRecordContextZeroStartNotCopied(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.RequestStart, time.Time{})
	got := usageRecordContext(parent, context.Background())
	if _, ok := got.Value(ctxkey.RequestStart).(time.Time); ok {
		t.Fatal("zero RequestStart should not be copied")
	}
}
