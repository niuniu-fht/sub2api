package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/tidwall/gjson"
)

// stubSyncStorage 捕获 Save 调用的内存对象存储。
type stubSyncStorage struct {
	mu     sync.Mutex
	saved  map[string][]byte
	types  map[string]string
}

func (s *stubSyncStorage) Save(ctx context.Context, key, contentType string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saved == nil {
		s.saved = map[string][]byte{}
		s.types = map[string]string{}
	}
	s.saved[key] = data
	s.types[key] = contentType
	return "https://cdn.example.com/" + key, nil
}

func withSyncUploaderForTest(storage ImageStorage) (restore func()) {
	orig := imageUploaderResolverForSync
	uploader := NewImageResultUploader(storage, "img/", 1<<20, nil)
	imageUploaderResolverForSync = func() (*ImageResultUploader, bool) { return uploader, true }
	return func() { imageUploaderResolverForSync = orig }
}

func b64ToURLPNG() string {
	return base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0})
}

func TestRewriteB64ToURL(t *testing.T) {
	store := &stubSyncStorage{}
	restore := withSyncUploaderForTest(store)
	defer restore()

	svc := &OpenAIGatewayService{}
	account := &Account{ID: 7, Extra: map[string]any{AccountExtraImagesB64ToURL: true}}
	body := []byte(`{"created":1,"data":[{"b64_json":"` + b64ToURLPNG() + `"}]}`)

	got := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	url := gjson.GetBytes(got, "data.0.url").String()
	if !strings.HasPrefix(url, "https://cdn.example.com/img/") {
		t.Fatalf("url=%s, want cdn prefix", url)
	}
	if gjson.GetBytes(got, "data.0.b64_json").String() == "" {
		t.Fatalf("b64_json must be preserved when hide is off")
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved=%d, want 1", len(store.saved))
	}
}

func TestRewriteB64ToURLWithHide(t *testing.T) {
	store := &stubSyncStorage{}
	restore := withSyncUploaderForTest(store)
	defer restore()

	svc := &OpenAIGatewayService{}
	account := &Account{ID: 7, Extra: map[string]any{
		AccountExtraImagesB64ToURL: true,
		AccountExtraImagesHideB64:  true,
	}}
	body := []byte(`{"created":1,"data":[{"b64_json":"` + b64ToURLPNG() + `"}]}`)

	got := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	if !strings.HasPrefix(gjson.GetBytes(got, "data.0.url").String(), "https://cdn.example.com/") {
		t.Fatalf("url missing after rewrite")
	}
	if gjson.GetBytes(got, "data.0.b64_json").Exists() {
		t.Fatalf("b64_json must be removed when hide is on")
	}
}

func TestRewriteURLOverrideAndDisabled(t *testing.T) {
	store := &stubSyncStorage{}
	restore := withSyncUploaderForTest(store)
	defer restore()

	// 上游已带 url:复用回填下载管线取字节,再以图床地址覆盖(同包 recorder 复刻公网响应)。
	upstream := &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", b64BackfillPNGBytes)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 7, Extra: map[string]any{AccountExtraImagesB64ToURL: true}}

	body := []byte(`{"created":1,"data":[{"url":"https://upstream.example.com/a.png"}]}`)
	got := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	if !strings.HasPrefix(gjson.GetBytes(got, "data.0.url").String(), "https://cdn.example.com/") {
		t.Fatalf("upstream url must be overridden with storage url, got %s", gjson.GetBytes(got, "data.0.url").String())
	}

	// 开关关闭:原样返回,不下载不覆盖。
	upstream2 := &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", b64BackfillPNGBytes)}
	svc2 := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream2}
	off := &Account{ID: 8}
	body2 := []byte(`{"created":1,"data":[{"url":"https://upstream.example.com/b.png"}]}`)
	got2 := svc2.rewriteOpenAIImagesB64ToURL(context.Background(), off, &OpenAIImagesRequest{}, body2)
	if gjson.GetBytes(got2, "data.0.url").String() != "https://upstream.example.com/b.png" {
		t.Fatalf("disabled account must keep upstream url")
	}
	if len(upstream2.requests) != 0 || len(store.saved) != 1 {
		t.Fatalf("disabled account must not download/upload")
	}
}
