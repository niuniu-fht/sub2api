package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
)

func TestGeminiImageAspectRatioFromSize(t *testing.T) {
	cases := map[string]string{
		"1024x1024": "1:1",
		"1536x1024": "3:2",
		"1024x1536": "2:3",
		"2048x2048": "1:1",
		"4096x4096": "1:1",
		"auto":      "",
		"1024":      "",
		"0x100":     "",
		"1366x768":  "",
	}
	for size, want := range cases {
		if got := geminiImageAspectRatioFromSize(size); got != want {
			t.Fatalf("geminiImageAspectRatioFromSize(%q) = %q, want %q", size, got, want)
		}
	}
}

func TestGeminiImageBridgeEnabled(t *testing.T) {
	base := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey}
	if geminiImageBridgeEnabled(base) {
		t.Fatal("bridge must be disabled without extra flag")
	}
	enabled := &Account{
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{AccountExtraGeminiImageBridge: true},
	}
	if !geminiImageBridgeEnabled(enabled) {
		t.Fatal("bridge must be enabled with extra flag")
	}
	openai := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{AccountExtraGeminiImageBridge: true},
	}
	if geminiImageBridgeEnabled(openai) {
		t.Fatal("bridge must not apply to non-gemini accounts")
	}
}

func TestBuildGeminiImageBridgePayload(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{}
	pngByte := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	parsed := &OpenAIImagesRequest{
		Prompt:   "a cat on the moon",
		N:        1,
		Size:     "1536x1024",
		SizeTier: "2K",
		Uploads: []OpenAIImagesUpload{{
			FieldName:   "image",
			FileName:    "ref.png",
			ContentType: "image/png",
			Data:        pngByte,
		}},
	}
	body, inlineCount, err := buildGeminiImageBridgePayload(context.Background(), svc, account, parsed)
	if err != nil {
		t.Fatalf("buildGeminiImageBridgePayload: %v", err)
	}
	if inlineCount != 1 {
		t.Fatalf("inlineCount = %d, want 1", inlineCount)
	}
	var payload geminiImageBridgePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload.Contents) != 1 || len(payload.Contents[0].Parts) != 2 {
		t.Fatalf("unexpected contents: %+v", payload.Contents)
	}
	if payload.Contents[0].Parts[0].Text != "a cat on the moon" {
		t.Fatalf("prompt part = %q", payload.Contents[0].Parts[0].Text)
	}
	inline := payload.Contents[0].Parts[1].InlineData
	if inline == nil || inline.MimeType != "image/png" {
		t.Fatalf("inline data part = %+v", payload.Contents[0].Parts[1])
	}
	if got, err := base64.StdEncoding.DecodeString(inline.Data); err != nil || string(got) != string(pngByte) {
		t.Fatalf("inline data roundtrip failed: %v", err)
	}
	if payload.GenerationConfig == nil {
		t.Fatal("generationConfig missing")
	}
	if len(payload.GenerationConfig.ResponseModalities) != 2 {
		t.Fatalf("responseModalities = %v", payload.GenerationConfig.ResponseModalities)
	}
	if payload.GenerationConfig.ImageConfig == nil ||
		payload.GenerationConfig.ImageConfig.AspectRatio != "3:2" ||
		payload.GenerationConfig.ImageConfig.ImageSize != "2K" {
		t.Fatalf("imageConfig = %+v", payload.GenerationConfig.ImageConfig)
	}
}

func TestParseGeminiImageBridgeResponse(t *testing.T) {
	imageB64 := base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))
	raw := []byte(`{
		"candidates": [{
			"content": {"parts": [
				{"text": "here is your image"},
				{"inlineData": {"mimeType": "image/png", "data": "` + imageB64 + `"}}
			]},
			"finishReason": "STOP"
		}],
		"usageMetadata": {"promptTokenCount": 12, "candidatesTokenCount": 560, "thoughtsTokenCount": 30, "totalTokenCount": 602}
	}`)
	images, usage, status, message, err := parseGeminiImageBridgeResponse(raw)
	if err != nil {
		t.Fatalf("parseGeminiImageBridgeResponse: %v", err)
	}
	if status != 0 || message != "" {
		t.Fatalf("unexpected error fields: status=%d message=%q", status, message)
	}
	if len(images) != 1 || images[0].MimeType != "image/png" || images[0].Base64 != imageB64 {
		t.Fatalf("images = %+v", images)
	}
	if usage.InputTokens != 12 || usage.OutputTokens != 590 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestParseGeminiImageBridgeResponseBlocked(t *testing.T) {
	raw := []byte(`{"promptFeedback": {"blockReason": "SAFETY"}, "usageMetadata": {"promptTokenCount": 9}}`)
	images, _, status, message, err := parseGeminiImageBridgeResponse(raw)
	if err != nil {
		t.Fatalf("parseGeminiImageBridgeResponse: %v", err)
	}
	if len(images) != 0 {
		t.Fatalf("expected no images, got %+v", images)
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if !strings.Contains(message, "SAFETY") {
		t.Fatalf("message = %q, want block reason", message)
	}
}

func TestParseGeminiImageBridgeResponseFinishReason(t *testing.T) {
	raw := []byte(`{"candidates": [{"content": {"parts": [{"text": "cannot generate"}]}, "finishReason": "PROHIBITED_CONTENT"}]}`)
	_, _, status, message, err := parseGeminiImageBridgeResponse(raw)
	if err != nil {
		t.Fatalf("parseGeminiImageBridgeResponse: %v", err)
	}
	if status != http.StatusBadRequest || !strings.Contains(message, "PROHIBITED_CONTENT") {
		t.Fatalf("status=%d message=%q", status, message)
	}
}

func TestGeminiImageBridgeStatusRetryable(t *testing.T) {
	for _, code := range []int{408, 429, 500, 502, 503, 504} {
		if !geminiImageBridgeStatusRetryable(code) {
			t.Fatalf("status %d should be retryable", code)
		}
	}
	for _, code := range []int{400, 401, 403, 404, 422, 451} {
		if geminiImageBridgeStatusRetryable(code) {
			t.Fatalf("status %d should not be retryable", code)
		}
	}
}

func newGeminiBridgeTestAccount() *Account {
	return &Account{
		ID:       77,
		Name:     "gemini-bridge",
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "AIza-test",
			"base_url": "https://generativelanguage.googleapis.com",
		},
		Extra: map[string]any{AccountExtraGeminiImageBridge: true},
	}
}

func geminiBridgeJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestForwardOpenAIImagesViaGemini(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	imageB64 := base64.StdEncoding.EncodeToString([]byte("fake-png-0"))
	upstream := &httpUpstreamRecorder{resp: geminiBridgeJSONResponse(`{
		"candidates": [{"content": {"parts": [{"inlineData": {"mimeType": "image/png", "data": "` + imageB64 + `"}}]}, "finishReason": "STOP"}],
		"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 100, "totalTokenCount": 110}
	}`)}
	svc := newOpenAIImagesTestService(upstream)
	account := newGeminiBridgeTestAccount()
	parsed := &OpenAIImagesRequest{
		Endpoint: openAIImagesGenerationsEndpoint,
		Model:    "gemini-3-pro-image-preview",
		Prompt:   "a cat",
		N:        1,
		Size:     "1024x1024",
		SizeTier: "1K",
	}

	result, err := svc.forwardOpenAIImagesViaGemini(context.Background(), c, account, parsed, "")
	if err != nil {
		t.Fatalf("forwardOpenAIImagesViaGemini: %v", err)
	}
	if upstream.lastReq == nil || upstream.lastReq.URL.Path != "/v1beta/models/gemini-3-pro-image-preview:generateContent" {
		t.Fatalf("upstream URL = %v", upstream.lastReq)
	}
	if got := upstream.lastReq.Header.Get("x-goog-api-key"); got != "AIza-test" {
		t.Fatalf("api key header = %q", got)
	}
	var sent geminiImageBridgePayload
	if err := json.Unmarshal(upstream.lastBody, &sent); err != nil {
		t.Fatalf("sent payload unmarshal: %v", err)
	}
	if sent.Contents[0].Parts[0].Text != "a cat" {
		t.Fatalf("sent prompt = %q", sent.Contents[0].Parts[0].Text)
	}
	if result.ImageCount != 1 || result.UpstreamModel != "gemini-3-pro-image-preview" || result.Stream {
		t.Fatalf("result = %+v", result)
	}
	if result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 100 {
		t.Fatalf("result usage = %+v", result.Usage)
	}

	var response struct {
		Created int64 `json:"created"`
		Data    []struct {
			B64JSON      string `json:"b64_json"`
			OutputFormat string `json:"output_format"`
		} `json:"data"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("client response unmarshal: %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != imageB64 || response.Data[0].OutputFormat != "png" {
		t.Fatalf("client response data = %+v", response.Data)
	}
	if response.Usage.TotalTokens != 110 {
		t.Fatalf("client response usage = %+v", response.Usage)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("client status = %d", rec.Code)
	}
}

func TestForwardOpenAIImagesViaGeminiRetryableFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error": {"code": 429, "message": "resource exhausted"}}`)),
	}}
	svc := newOpenAIImagesTestService(upstream)
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gemini-3-pro-image-preview", Prompt: "a cat", N: 1}

	_, err := svc.forwardOpenAIImagesViaGemini(context.Background(), c, newGeminiBridgeTestAccount(), parsed, "")
	if err == nil {
		t.Fatal("expected failover error")
	}
	if _, ok := err.(*UpstreamFailoverError); !ok {
		t.Fatalf("expected UpstreamFailoverError, got %T: %v", err, err)
	}
}

func TestForwardOpenAIImagesViaGeminiUserErrorPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 透传规则:平台 gemini、命中关键词,原样透传状态码与信息。
	rule := &model.ErrorPassthroughRule{
		ID:              1,
		Enabled:         true,
		Priority:        0,
		ErrorCodes:      []int{400},
		Keywords:        []string{"content policy"},
		MatchMode:       model.MatchModeAny,
		Platforms:       []string{PlatformGemini},
		PassthroughCode: true,
		PassthroughBody: true,
	}
	ptSvc := NewErrorPassthroughService(&stubPassthroughRepo{rules: []*model.ErrorPassthroughRule{rule}}, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	BindErrorPassthroughService(c, ptSvc)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error": {"code": 400, "message": "prompt violates content policy"}}`)),
	}}
	svc := newOpenAIImagesTestService(upstream)
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gemini-3-pro-image-preview", Prompt: "a cat", N: 1}

	_, err := svc.forwardOpenAIImagesViaGemini(context.Background(), c, newGeminiBridgeTestAccount(), parsed, "")
	if err == nil {
		t.Fatal("expected passthrough upstream error")
	}
	upErr, ok := err.(*OpenAIImagesUpstreamError)
	if !ok {
		t.Fatalf("expected OpenAIImagesUpstreamError, got %T: %v", err, err)
	}
	if upErr.StatusCode != http.StatusBadRequest || !strings.Contains(upErr.Message, "content policy") {
		t.Fatalf("passthrough error = %+v", upErr)
	}
}

func TestForwardOpenAIImagesViaGeminiBlockedWithoutImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	upstream := &httpUpstreamRecorder{resp: geminiBridgeJSONResponse(`{
		"promptFeedback": {"blockReason": "BLOCKLIST"},
		"usageMetadata": {"promptTokenCount": 7}
	}`)}
	svc := newOpenAIImagesTestService(upstream)
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gemini-3-pro-image-preview", Prompt: "a cat", N: 1}

	_, err := svc.forwardOpenAIImagesViaGemini(context.Background(), c, newGeminiBridgeTestAccount(), parsed, "")
	if err == nil {
		t.Fatal("expected blocked upstream error")
	}
	upErr, ok := err.(*OpenAIImagesUpstreamError)
	if !ok {
		t.Fatalf("expected OpenAIImagesUpstreamError, got %T: %v", err, err)
	}
	if upErr.StatusCode != http.StatusBadRequest || !strings.Contains(upErr.Message, "BLOCKLIST") {
		t.Fatalf("blocked error = %+v", upErr)
	}
}

func TestForwardOpenAIImagesViaGeminiStreamRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	svc := newOpenAIImagesTestService(&httpUpstreamRecorder{})
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gemini-3-pro-image-preview", Prompt: "a cat", N: 1, Stream: true}

	_, err := svc.forwardOpenAIImagesViaGemini(context.Background(), c, newGeminiBridgeTestAccount(), parsed, "")
	if err == nil {
		t.Fatal("expected stream rejection")
	}
	upErr, ok := err.(*OpenAIImagesUpstreamError)
	if !ok || upErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 upstream error, got %T: %v", err, err)
	}
}

// stubPassthroughRepo 透传规则仓库桩:仅返回预置规则。
type stubPassthroughRepo struct {
	rules []*model.ErrorPassthroughRule
}

func (r *stubPassthroughRepo) List(_ context.Context) ([]*model.ErrorPassthroughRule, error) {
	return r.rules, nil
}

func (r *stubPassthroughRepo) GetByID(_ context.Context, id int64) (*model.ErrorPassthroughRule, error) {
	for _, rule := range r.rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return nil, nil
}

func (r *stubPassthroughRepo) Create(_ context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	return rule, nil
}

func (r *stubPassthroughRepo) Update(_ context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	return rule, nil
}

func (r *stubPassthroughRepo) Delete(_ context.Context, _ int64) error { return nil }
