package repository

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ─── 测试桩：记录调用的假主存储 ─────────────────────────────

type stubStorage struct {
	url    string
	err    error
	called int
}

func (s *stubStorage) Save(ctx context.Context, key, contentType string, data []byte) (string, error) {
	s.called++
	if s.err != nil {
		return "", s.err
	}
	return s.url, nil
}

func beijingTime(h, m int) time.Time {
	return time.Date(2026, 11, 8, h, m, 0, 0, beijingTz)
}

// ─── 窗口解析 ───────────────────────────────────────────────

func TestInR2HubUploadWindow(t *testing.T) {
	cases := []struct {
		window string
		now    time.Time
		want   bool
	}{
		{"20:00-05:00", beijingTime(21, 0), true},  // 夜间在窗口内
		{"20:00-05:00", beijingTime(3, 0), true},   // 跨午夜凌晨在窗口内
		{"20:00-05:00", beijingTime(5, 0), false},  // 结束时刻不含
		{"20:00-05:00", beijingTime(12, 0), false}, // 白天不在窗口
		{"03:00-03:10", beijingTime(21, 0), false}, // 窗口外
		{"03:00-03:10", beijingTime(3, 5), true},   // 窗口内
		{"", beijingTime(12, 0), true},             // 空 = 全天
		{"abc", beijingTime(12, 0), true},          // 非法 = 全天
		{"25:99-07:00", beijingTime(12, 0), true},  // 越界 = 全天
	}
	for _, tc := range cases {
		if got := InR2HubUploadWindow(tc.window, tc.now); got != tc.want {
			t.Errorf("InR2HubUploadWindow(%q, %02d:%02d) = %v, want %v",
				tc.window, tc.now.Hour(), tc.now.Minute(), got, tc.want)
		}
	}
}

// ─── r2hub HTTP 客户端 ───────────────────────────────────────

func TestR2HubImageStorageSave(t *testing.T) {
	t.Run("success returns url", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/upload" {
				t.Errorf("path = %q, want /upload", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer r2h_test" {
				t.Errorf("auth = %q", got)
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
			}
			_, _ = w.Write([]byte(`{"id":1,"url":"https://pub-x.r2.dev/images/20261108/a.png"}`))
		}))
		defer srv.Close()

		storage := NewR2HubImageStorage(config.ImageStorageR2HubConfig{
			BaseURL: srv.URL, APIKey: "r2h_test", TimeoutSecs: 5,
		})
		url, err := storage.Save(context.Background(), "img/task-1-0.png", "image/png", []byte("data"))
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
		if url != "https://pub-x.r2.dev/images/20261108/a.png" {
			t.Errorf("url = %q", url)
		}
	})

	t.Run("http error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		storage := NewR2HubImageStorage(config.ImageStorageR2HubConfig{BaseURL: srv.URL, APIKey: "bad"})
		if _, err := storage.Save(context.Background(), "k.png", "image/png", []byte("d")); err == nil {
			t.Fatal("expected error for 401")
		}
	})

	t.Run("missing url field", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"id":2}`))
		}))
		defer srv.Close()
		storage := NewR2HubImageStorage(config.ImageStorageR2HubConfig{BaseURL: srv.URL, APIKey: "k"})
		if _, err := storage.Save(context.Background(), "k.png", "image/png", []byte("d")); err == nil {
			t.Fatal("expected error for missing url")
		}
	})
}

// ─── 窗口装饰器 ──────────────────────────────────────────────

func TestWrapWindowedR2Hub(t *testing.T) {
	primary := &stubStorage{url: "https://cdn.code2alita.com/img/k.png"}
	r2hubURL := "https://pub-x.r2.dev/images/20261108/a.png"
	fail := errors.New("r2hub down")

	makeDecorator := func(window string, r2hubErr error, r2hubURL string) *windowedR2HubStorage {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if r2hubErr != nil {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"url":%q}`, r2hubURL)))
		}))
		t.Cleanup(srv.Close)
		decorator := WrapWindowedR2Hub(primary, config.ImageStorageR2HubConfig{
			Enabled: true, BaseURL: srv.URL, APIKey: "r2h_t", UploadWindow: window,
		}).(*windowedR2HubStorage)
		return decorator
	}

	t.Run("in-window success returns r2hub url, primary untouched", func(t *testing.T) {
		primary.called = 0
		d := makeDecorator("20:00-05:00", nil, r2hubURL)
		d.nowFunc = func() time.Time { return beijingTime(21, 0) }
		url, err := d.Save(context.Background(), "k.png", "image/png", []byte("d"))
		if err != nil || url != r2hubURL {
			t.Fatalf("url=%q err=%v", url, err)
		}
		if primary.called != 0 {
			t.Errorf("primary called %d times, want 0", primary.called)
		}
	})

	t.Run("in-window r2hub failure falls back to primary", func(t *testing.T) {
		d := makeDecorator("20:00-05:00", fail, "")
		d.nowFunc = func() time.Time { return beijingTime(21, 0) }
		url, err := d.Save(context.Background(), "k.png", "image/png", []byte("d"))
		if err != nil || url != primary.url {
			t.Fatalf("url=%q err=%v", url, err)
		}
		if primary.called != 1 {
			t.Errorf("primary called %d times, want 1", primary.called)
		}
	})

	t.Run("out-of-window goes straight to primary", func(t *testing.T) {
		primary.called = 0
		d := makeDecorator("03:00-03:10", nil, r2hubURL)
		d.nowFunc = func() time.Time { return beijingTime(21, 0) }
		url, err := d.Save(context.Background(), "k.png", "image/png", []byte("d"))
		if err != nil || url != primary.url {
			t.Fatalf("url=%q err=%v", url, err)
		}
		if primary.called != 1 {
			t.Errorf("primary called %d times, want 1", primary.called)
		}
	})

	t.Run("primary error still surfaces", func(t *testing.T) {
		failingPrimary := &stubStorage{err: errors.New("minio down")}
		d := makeDecorator("03:00-03:10", nil, r2hubURL)
		d.primary = failingPrimary
		d.nowFunc = func() time.Time { return beijingTime(21, 0) }
		if _, err := d.Save(context.Background(), "k.png", "image/png", []byte("d")); err == nil {
			t.Fatal("expected primary error to surface")
		}
	})
}

func TestWrapWindowedR2HubDisabledOrIncomplete(t *testing.T) {
	primary := &stubStorage{url: "https://primary/k.png"}
	// 未启用：原样返回 primary
	wrapped := WrapWindowedR2Hub(primary, config.ImageStorageR2HubConfig{Enabled: false})
	if _, ok := wrapped.(*windowedR2HubStorage); ok {
		t.Error("disabled config should return primary unwrapped")
	}
	// 启用但凭证不全：同样原样返回 primary
	wrapped = WrapWindowedR2Hub(primary, config.ImageStorageR2HubConfig{Enabled: true, BaseURL: "", APIKey: ""})
	if _, ok := wrapped.(*windowedR2HubStorage); ok {
		t.Error("incomplete config should return primary unwrapped")
	}
	// 且两者都可用
	if url, err := wrapped.Save(context.Background(), "k.png", "image/png", []byte("d")); err != nil || url != primary.url {
		t.Fatalf("passthrough broken: url=%q err=%v", url, err)
	}
}

// 编译期确认装饰器满足接口
var _ service.ImageStorage = (*windowedR2HubStorage)(nil)
