package service

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func attemptAccount(id int64, name string, platform string) *Account {
	return &Account{ID: id, Name: name, Platform: platform}
}

func TestAppendAccountAttemptRecordsSuccessAndFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	start := time.Now()
	AppendAccountAttempt(c, attemptAccount(1, "a1", "gemini"), start, 400, "quota exceeded", false)
	AppendAccountAttempt(c, attemptAccount(2, "a2", "gemini"), start, 200, "", true)

	attempts := AccountAttemptsFromContext(c)
	require.Len(t, attempts, 2)
	require.Equal(t, int64(1), attempts[0].AccountID)
	require.Equal(t, 400, attempts[0].StatusCode)
	require.Contains(t, attempts[0].Error, "quota exceeded")
	require.False(t, attempts[0].Success)
	require.True(t, attempts[1].Success)
	require.Equal(t, 200, attempts[1].StatusCode)
	require.Empty(t, attempts[1].Error)
}

func TestAppendAccountAttemptMergesSameAccountRetries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	start := time.Now()
	AppendAccountAttempt(c, attemptAccount(7, "retry", "gemini"), start, 400, "first error", false)
	AppendAccountAttempt(c, attemptAccount(7, "retry", "gemini"), start, 503, "second error", false)
	AppendAccountAttempt(c, attemptAccount(7, "retry", "gemini"), start, 200, "", true)

	attempts := AccountAttemptsFromContext(c)
	require.Len(t, attempts, 1, "same-account retries must merge into a single chain entry")
	merged := attempts[0]
	require.Equal(t, int64(7), merged.AccountID)
	require.True(t, merged.Success, "final outcome wins")
	require.Equal(t, 200, merged.StatusCode)
	require.Empty(t, merged.Error, "error cleared on final success")
	require.GreaterOrEqual(t, merged.DurationMs, int64(0), "durations accumulate across retries")
}

func TestAppendAccountAttemptNilContextAndLongErrorTruncated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	AppendAccountAttempt(nil, attemptAccount(1, "x", "gemini"), time.Now(), 400, "boom", false)

	c, _ := gin.CreateTestContext(nil)
	longErr := make([]byte, accountAttemptErrorMaxLen+100)
	for i := range longErr {
		longErr[i] = 'a'
	}
	AppendAccountAttempt(c, attemptAccount(1, "x", "gemini"), time.Now(), 400, string(longErr), false)

	attempts := AccountAttemptsFromContext(c)
	require.Len(t, attempts, 1)
	require.Len(t, attempts[0].Error, accountAttemptErrorMaxLen)
}
