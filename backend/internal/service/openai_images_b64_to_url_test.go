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

	got, err := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
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

	got, err := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
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
	got, err := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(gjson.GetBytes(got, "data.0.url").String(), "https://cdn.example.com/") {
		t.Fatalf("upstream url must be overridden with storage url, got %s", gjson.GetBytes(got, "data.0.url").String())
	}

	// 开关关闭:原样返回,不下载不覆盖。
	upstream2 := &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", b64BackfillPNGBytes)}
	svc2 := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream2}
	off := &Account{ID: 8}
	body2 := []byte(`{"created":1,"data":[{"url":"https://upstream.example.com/b.png"}]}`)
	got2, err2 := svc2.rewriteOpenAIImagesB64ToURL(context.Background(), off, &OpenAIImagesRequest{}, body2)
	if err2 != nil {
		t.Fatalf("disabled account must not error: %v", err2)
	}
	if gjson.GetBytes(got2, "data.0.url").String() != "https://upstream.example.com/b.png" {
		t.Fatalf("disabled account must keep upstream url")
	}
	if len(upstream2.requests) != 0 || len(store.saved) != 1 {
		t.Fatalf("disabled account must not download/upload")
	}
}

func TestRewriteKeysUniquePerRequest(t *testing.T) {
	// 串图回归:同一账号两次请求,对象键必须不同(否则第二次覆盖第一次,URL 串图)。
	store := &stubSyncStorage{}
	restore := withSyncUploaderForTest(store)
	defer restore()

	svc := &OpenAIGatewayService{}
	account := &Account{ID: 203, Extra: map[string]any{AccountExtraImagesB64ToURL: true}}
	body := []byte(`{"created":1,"data":[{"b64_json":"` + b64ToURLPNG() + `"}]}`)

	got1, err1 := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	if err1 != nil {
		t.Fatalf("unexpected err: %v", err1)
	}
	got2, err2 := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	if err2 != nil {
		t.Fatalf("unexpected err: %v", err2)
	}

	url1 := gjson.GetBytes(got1, "data.0.url").String()
	url2 := gjson.GetBytes(got2, "data.0.url").String()
	if url1 == "" || url2 == "" {
		t.Fatalf("urls missing: %q %q", url1, url2)
	}
	if url1 == url2 {
		t.Fatalf("FATAL 串图: 两次请求返回同一 URL %s — 第二次会覆盖第一次的图", url1)
	}
	if len(store.saved) != 2 {
		t.Fatalf("saved=%d, want 2 distinct objects", len(store.saved))
	}
	keys := make([]string, 0)
	for k := range store.saved {
		keys = append(keys, k)
	}
	if keys[0] == keys[1] {
		t.Fatalf("FATAL: 同一账号两次请求写了同一个 key %s", keys[0])
	}
}

// failingSyncStorage 恒定失败的存储:验证「必须转存」语义——失败时报错而非保留上游 URL/b64。
type failingSyncStorage struct {
	attempts int
}

func (f *failingSyncStorage) Save(ctx context.Context, key, contentType string, data []byte) (string, error) {
	f.attempts++
	return "", context.DeadlineExceeded
}

func TestRewriteB64ToURLStorageFailureIsHardError(t *testing.T) {
	// 开关开启后转存是硬性承诺:上传重试耗尽仍失败 → 返回错误,绝不原样返回响应。
	store := &failingSyncStorage{}
	restore := withSyncUploaderForTest(store)
	defer restore()

	svc := &OpenAIGatewayService{}
	account := &Account{ID: 9, Extra: map[string]any{AccountExtraImagesB64ToURL: true}}
	body := []byte(`{"created":1,"data":[{"b64_json":"` + b64ToURLPNG() + `"}]}`)

	got, err := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	if err == nil {
		t.Fatalf("storage failure must surface as error, got body %s", string(got))
	}
	if got != nil {
		t.Fatalf("failed rewrite must not return a body (would leak un-rehosted data): %s", string(got))
	}
	if !strings.Contains(err.Error(), "image re-host failed") {
		t.Fatalf("err=%v, want re-host failure message", err)
	}
	if store.attempts != b64ToURLRetryAttempts {
		t.Fatalf("save attempts=%d, want %d (one retry)", store.attempts, b64ToURLRetryAttempts)
	}
}

func TestRewriteB64ToURLFetchFailureIsHardError(t *testing.T) {
	// 上游 url 下载失败(如超限/网络):同样报错,不保留上游 url。
	store := &stubSyncStorage{}
	restore := withSyncUploaderForTest(store)
	defer restore()

	upstream := &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusInternalServerError, "application/json", []byte(`{"error":"boom"}`))}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 10, Extra: map[string]any{AccountExtraImagesB64ToURL: true}}
	body := []byte(`{"created":1,"data":[{"url":"https://upstream.example.com/c.png"}]}`)

	got, err := svc.rewriteOpenAIImagesB64ToURL(context.Background(), account, &OpenAIImagesRequest{}, body)
	if err == nil {
		t.Fatalf("download failure must surface as error, got body %s", string(got))
	}
	if got != nil {
		t.Fatalf("failed rewrite must not return a body with upstream url: %s", string(got))
	}
	if strings.Contains(gjson.Get(`{"u":"x"}`, "u").String(), "") && len(store.saved) != 0 {
		t.Fatalf("nothing must be uploaded on download failure")
	}
}
