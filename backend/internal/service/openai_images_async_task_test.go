package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// recordingUpstream 把请求重写到目标测试服务器,并保留一份请求快照。
type recordingUpstream struct {
	target   *httptest.Server
	requests []*http.Request
}

func (r *recordingUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	r.requests = append(r.requests, req)
	req.URL.Scheme = "http"
	req.URL.Host = r.target.Listener.Addr().String()
	return http.DefaultClient.Do(req)
}

func (r *recordingUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return r.Do(req, proxyURL, accountID, accountConcurrency)
}

func asyncTaskTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	return c, rec
}

func asyncTaskTestConfig(serverURL string) (*asyncTaskUpstreamConfig, error) {
	cfgRaw := map[string]any{
		"base_url":              serverURL,
		"api_key":               "sk-test",
		"create_path":           "/v2/images/tasks",
		"create_body":           map[string]any{"model": "gpt-image-2", "channelId": "gpt-image-default", "params": map[string]any{"prompt": "{{prompt}}", "size": "{{size}}", "quality": "{{quality}}", "count": "{{n}}"}},
		"task_id_path":          "task_id",
		"status_path":           "/v2/images/tasks/{{task_id}}",
		"status_path_value":     "status",
		"success_value":         "succeeded",
		"failure_value":         "failed",
		"image_urls_path":       "output.images.#.url",
		"error_message_path":    "error.message",
		"poll_interval_seconds": 0,
		"timeout_seconds":       10,
	}
	blob, err := json.Marshal(cfgRaw)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(blob, &raw); err != nil {
		return nil, err
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_image_async_task": raw}}
	return getAsyncTaskUpstreamConfig(account)
}

func TestAsyncTaskUpstreamConfigMissingFields(t *testing.T) {
	if _, err := getAsyncTaskUpstreamConfig(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{
		"openai_image_async_task": map[string]any{"base_url": "https://x"},
	}}); err == nil {
		t.Fatalf("expected error for incomplete config")
	}
	if cfg, err := getAsyncTaskUpstreamConfig(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}); err != nil || cfg != nil {
		t.Fatalf("expected nil config for account without async config, got %v / %v", cfg, err)
	}
}

func TestForwardOpenAIImagesViaAsyncTaskSuccess(t *testing.T) {
	createCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/images/tasks":
			createCalls++
			if r.Header.Get("Authorization") != "Bearer sk-test" {
				t.Errorf("missing bearer auth: %s", r.Header.Get("Authorization"))
			}
			body, _ := io.ReadAll(r.Body)
			if gjson.GetBytes(body, "params.prompt").String() != "a cat" {
				t.Errorf("template prompt not resolved: %s", body)
			}
			if gjson.GetBytes(body, "params.count").Int() != 1 {
				t.Errorf("template count not typed: %s", body)
			}
			_, _ = w.Write([]byte(`{"task_id":"imgtask_abc","status":"pending"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v2/images/tasks/imgtask_abc":
			_, _ = w.Write([]byte(`{"status":"succeeded","output":{"images":[{"url":"http://` + r.Host + `/img/0.png"}]}}`))
		case r.URL.Path == "/img/0.png":
			_, _ = w.Write([]byte("fakepng"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	up := &recordingUpstream{target: upstream}
	svc := &OpenAIGatewayService{httpUpstream: up}
	cfg, err := asyncTaskTestConfig(upstream.URL)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	parsed := &OpenAIImagesRequest{Model: "gpt-image-2", Prompt: "a cat", N: 1, Size: "1024x1024", Quality: "high", ResponseFormat: "url"}
	c, rec := asyncTaskTestContext()

	result, err := svc.forwardOpenAIImagesViaAsyncTask(context.Background(), c, &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 3}, []byte(`{}`), parsed, "", cfg)
	if err != nil {
		if fe, ok := err.(*UpstreamFailoverError); ok {
			t.Fatalf("forward failover: status=%d body=%s", fe.StatusCode, string(fe.ResponseBody))
		}
		t.Fatalf("forward: %v", err)
	}
	if result.ImageCount != 1 || result.ImageSize != "1024x1024" || result.ImageQuality != "high" {
		t.Fatalf("billing fields wrong: %+v", result)
	}
	if createCalls != 1 {
		t.Fatalf("create calls=%d, want 1", createCalls)
	}
	body := rec.Body.Bytes()
	wantURL := upstream.URL + "/img/0.png"
	if gjson.GetBytes(body, "data.0.url").String() != wantURL {
		t.Fatalf("data.0.url=%s, want %s", gjson.GetBytes(body, "data.0.url").String(), wantURL)
	}
}

func TestForwardOpenAIImagesViaAsyncTaskFailureSwitches(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/v2/images/tasks" {
			_, _ = w.Write([]byte(`{"task_id":"imgtask_f","status":"pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"failed","error":{"message":"generation status FAILED"}}`))
	}))
	defer upstream.Close()

	up := &recordingUpstream{target: upstream}
	svc := &OpenAIGatewayService{httpUpstream: up}
	cfg, err := asyncTaskTestConfig(upstream.URL)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	c, _ := asyncTaskTestContext()

	_, err = svc.forwardOpenAIImagesViaAsyncTask(context.Background(), c, &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 3}, []byte(`{}`), &OpenAIImagesRequest{Model: "gpt-image-2", Prompt: "p", N: 1}, "", cfg)
	failoverErr, ok := err.(*UpstreamFailoverError)
	if !ok {
		t.Fatalf("want UpstreamFailoverError for failed task, got %T %v", err, err)
	}
	if failoverErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502", failoverErr.StatusCode)
	}
}

func TestForwardOpenAIImagesViaAsyncTaskCreate400Passthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad params"}}`))
	}))
	defer upstream.Close()

	up := &recordingUpstream{target: upstream}
	svc := &OpenAIGatewayService{httpUpstream: up}
	cfg, err := asyncTaskTestConfig(upstream.URL)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	c, _ := asyncTaskTestContext()

	_, err = svc.forwardOpenAIImagesViaAsyncTask(context.Background(), c, &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 3}, []byte(`{}`), &OpenAIImagesRequest{Model: "gpt-image-2", Prompt: "p", N: 1}, "", cfg)
	upErr, ok := err.(*OpenAIImagesUpstreamError)
	if !ok {
		t.Fatalf("want OpenAIImagesUpstreamError for 400 create, got %T %v", err, err)
	}
	if upErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", upErr.StatusCode)
	}
}
