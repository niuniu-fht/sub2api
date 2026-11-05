//go:build unit

package handler

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestRecordImageAccountAttemptExtractsRealUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	start := time.Now()

	err := &service.UpstreamFailoverError{
		StatusCode: 400,
		ResponseBody: []byte(`{"error":{"code":"API_KEY_INVALID","message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`),
	}
	recordImageAccountAttempt(c, &service.Account{ID: 1, Name: "a1", Platform: "gemini"}, start, err)

	attempts := service.AccountAttemptsFromContext(c)
	require.Len(t, attempts, 1)
	require.Equal(t, 400, attempts[0].StatusCode)
	require.Contains(t, attempts[0].Error, "API key not valid")
	require.NotContains(t, attempts[0].Error, "(failover)", "must not surface the wrapped synthetic message")
}

func TestRecordGeminiAccountAttemptExtractsRealUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	start := time.Now()

	err := &service.UpstreamFailoverError{
		StatusCode:   429,
		ResponseBody: []byte(`{"error":{"code":429,"message":"Resource has been exhausted","status":"RESOURCE_EXHAUSTED"}}`),
	}
	recordGeminiAccountAttempt(c, &service.Account{ID: 2, Name: "a2", Platform: "gemini"}, start, err)

	attempts := service.AccountAttemptsFromContext(c)
	require.Len(t, attempts, 1)
	require.Equal(t, 429, attempts[0].StatusCode)
	require.Contains(t, attempts[0].Error, "Resource has been exhausted")
}

func TestRecordImageAccountAttemptFallsBackToRawBodyWhenNoMessageField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	start := time.Now()

	err := &service.UpstreamFailoverError{
		StatusCode:   502,
		ResponseBody: []byte(`<html>bad gateway from origin-7</html>`),
	}
	recordImageAccountAttempt(c, &service.Account{ID: 3, Name: "a3", Platform: "gemini"}, start, err)

	attempts := service.AccountAttemptsFromContext(c)
	require.Len(t, attempts, 1)
	require.Contains(t, attempts[0].Error, "bad gateway from origin-7", "raw body snippet shown when no error.message field")
}

func TestUpstreamAttemptErrorMessageTransportFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &service.Account{ID: 9, Name: "transport", Platform: "gemini"}

	// 无 body、无 ops 事件：回退错误字符串。
	require.Equal(t, "dial tcp: lookup leoapp: no such host",
		upstreamAttemptErrorMessage(nil, account, nil, errString("dial tcp: lookup leoapp: no such host")))
	require.Equal(t, "", upstreamAttemptErrorMessage(nil, account, nil, nil))

	// 传输层占位文案：从 ops 事件还原真实原因（DNS/超时/连接拒绝等）。
	c, _ := gin.CreateTestContext(nil)
	c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{
		{AccountID: 9, Kind: "request_error", Message: `Post "http://leoapp:8099/v1beta": dial tcp: lookup leoapp: no such host`},
	})
	synthetic := &service.UpstreamFailoverError{
		StatusCode:   502,
		ResponseBody: []byte(`{"error":{"code":502,"message":"Upstream request failed","status":"INTERNAL"}}`),
	}
	require.Equal(t, `Post "http://leoapp:8099/v1beta": dial tcp: lookup leoapp: no such host`,
		upstreamAttemptErrorMessage(c, account, synthetic.ResponseBody, synthetic))
}

type errString string

func (e errString) Error() string { return string(e) }
