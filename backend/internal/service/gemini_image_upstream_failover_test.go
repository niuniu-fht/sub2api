//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 生产案例 8a3b0f82：prompt-hubs(262) 对 nano-banana-pro 返回 404 model_not_found
// （其自定义错误码未命中 → ErrorPolicySkipped），旧逻辑 404 不可换号导致图片
// 路由链 [262,168] 的第二个账号从未被尝试。图片 scope 下放宽为"除 400 外一律换号"。

func imageScopeCtx() context.Context {
	return WithImageBillingErrorFailoverScope(context.Background())
}

const geminiImageFailoverTestBody = `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`

func newImageFailoverSvc(status int, body string) *GeminiMessagesCompatService {
	return &GeminiMessagesCompatService{
		httpUpstream: &geminiCompatHTTPUpstreamStub{
			response: &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			},
		},
		cfg:              &config.Config{},
		rateLimitService: NewRateLimitService(&errorPolicyRepoStub{}, nil, &config.Config{}, nil, nil),
	}
}

func plainGeminiKeyAccount() *Account {
	return &Account{
		ID:          251,
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}
}

func newImageFailoverContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	c, rec := newGeminiNativeTestContext(t)
	c.Request = c.Request.WithContext(imageScopeCtx())
	return c, rec
}

// 图片 scope + 404（自定义错误码未命中 → Skipped 分支，生产案例路径）→ 换号。
func TestGeminiForwardNative_ImageScope404CustomCodesSkippedFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	notFoundBody := `{"error":{"code":"model_not_found","message":"Model nano-banana-pro is currently unavailable in group pure-banana","type":"new_api_error"}}`
	svc := newImageFailoverSvc(http.StatusNotFound, notFoundBody)
	c, _ := newImageFailoverContext(t)

	result, err := svc.ForwardNative(imageScopeCtx(), c, geminiCustomCodesAPIKeyAccount(),
		"gemini-3-pro-image", "generateContent", false, []byte(geminiImageFailoverTestBody))

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "图片 scope 下 404 必须换号, got %v", err)
	require.Equal(t, http.StatusNotFound, failoverErr.StatusCode)
	require.False(t, c.Writer.Written(), "换号场景不应写客户端响应")
	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok, "应记录 failover 事件")
	events := raw.([]*OpsUpstreamErrorEvent)
	require.Len(t, events, 1)
	require.Equal(t, "failover", events[0].Kind)
	require.Equal(t, http.StatusNotFound, events[0].UpstreamStatusCode)
}

// 图片 scope + 404（普通账号，ErrorPolicyNone 分支）→ 换号。
func TestGeminiForwardNative_ImageScope404PlainAccountFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newImageFailoverSvc(http.StatusNotFound, `{"error":{"message":"not found"}}`)
	c, _ := newImageFailoverContext(t)

	result, err := svc.ForwardNative(imageScopeCtx(), c, plainGeminiKeyAccount(),
		"gemini-3-pro-image", "generateContent", false, []byte(geminiImageFailoverTestBody))

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "图片 scope 下 404 必须换号, got %v", err)
	require.Equal(t, http.StatusNotFound, failoverErr.StatusCode)
	require.False(t, c.Writer.Written())
}

// 图片 scope + 400 → 唯一不换号的状态：按上游真实响应透传给客户端。
func TestGeminiForwardNative_ImageScope400DoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	badRequestBody := `{"error":{"code":"INVALID_ARGUMENT","message":"prompt blocked"}}`
	svc := newImageFailoverSvc(http.StatusBadRequest, badRequestBody)
	c, rec := newImageFailoverContext(t)

	result, err := svc.ForwardNative(imageScopeCtx(), c, plainGeminiKeyAccount(),
		"gemini-3-pro-image", "generateContent", false, []byte(geminiImageFailoverTestBody))

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "图片 scope 下 400 不应换号")
	require.True(t, c.Writer.Written(), "400 应透传上游响应")
	require.Contains(t, rec.Body.String(), "prompt blocked")
}

// 非图片请求维持原语义：404 不换号（聊天路径行为不变）。
func TestGeminiForwardNative_NoScope404KeepsLegacyBehavior(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newImageFailoverSvc(http.StatusNotFound, `{"error":{"message":"not found"}}`)
	c, rec := newGeminiNativeTestContext(t)
	_ = rec

	result, err := svc.ForwardNative(context.Background(), c, plainGeminiKeyAccount(),
		"gemini-2.5-flash", "generateContent", false, []byte(geminiImageFailoverTestBody))

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "非图片 404 维持不换号")
	require.True(t, c.Writer.Written())
}

func TestShouldFailoverGeminiImageUpstreamStatus(t *testing.T) {
	scoped := imageScopeCtx()
	require.False(t, shouldFailoverGeminiImageUpstreamStatus(scoped, http.StatusBadRequest))
	require.True(t, shouldFailoverGeminiImageUpstreamStatus(scoped, http.StatusNotFound))
	require.True(t, shouldFailoverGeminiImageUpstreamStatus(scoped, http.StatusRequestTimeout))
	require.True(t, shouldFailoverGeminiImageUpstreamStatus(scoped, http.StatusBadGateway))
	require.False(t, shouldFailoverGeminiImageUpstreamStatus(context.Background(), http.StatusNotFound))
	require.False(t, shouldFailoverGeminiImageUpstreamStatus(context.Background(), http.StatusBadGateway))
}
