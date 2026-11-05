package service

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AccountAttempt 一次请求内的单账号尝试记录（账号尝试链）：
// handler 层 failover 循环在每次账号尝试结束（成功或失败）时追加，
// 随最终 usage_log 行落库（仅链长>1 即发生过账号切换时持久化）。
// 同一账号的原地重试合并为一条：错误取最后一次、耗时累加，
// 使链上每个账号恰好一行，次数>1 即等价于"发生过账号切换"。
type AccountAttempt struct {
	AtUnixMs    int64  `json:"at_unix_ms"`
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name,omitempty"`
	Platform    string `json:"platform,omitempty"`
	// StatusCode 上游 HTTP 状态码；0 表示未到达上游（传输/凭据错误）。
	StatusCode int `json:"status_code,omitempty"`
	// DurationMs 该账号上的累计尝试耗时（含同账号原地重试）。
	DurationMs int64 `json:"duration_ms"`
	// Error 最后一次失败的错误摘要（sanitize + 截断），成功时为空。
	Error   string `json:"error,omitempty"`
	Success bool   `json:"success"`
}

const accountAttemptErrorMaxLen = 500

const accountAttemptsContextKey = "service.account_attempts"

// TransportFailoverPlaceholderMessage 是三条传输层失败路径（gemini/openai/claude，
// 见 *_transport_error.go）挂到 failover 错误 body 上的占位文案。占位文案不含真实
// 原因，账号尝试链遇到它时回退到 ops 错误事件里该账号的真实错误。
const TransportFailoverPlaceholderMessage = "Upstream request failed"

// LastUpstreamEventMessageForAccount 返回该账号最近一次失败事件的真实错误消息（已脱敏）。
// 传输层失败时 failover 错误的 body 是合成占位文案，真实原因（DNS 解析失败/超时/
// 连接拒绝等）只记录在请求的 ops 错误事件里；账号尝试链用它还原每次尝试的真实错误。
func LastUpstreamEventMessageForAccount(c *gin.Context, accountID int64) string {
	if c == nil {
		return ""
	}
	var events []*OpsUpstreamErrorEvent
	if v, ok := c.Get(OpsUpstreamErrorsKey); ok {
		events, _ = v.([]*OpsUpstreamErrorEvent)
	}
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev == nil || ev.AccountID != accountID || ev.Message == "" {
			continue
		}
		return ev.Message
	}
	return ""
}

// AppendAccountAttempt 在 gin ctx 上记录一次账号尝试。
// startedAt 为该次尝试（该账号被选中后）的起始时刻；success=false 时
// statusCode/errMsg 描述失败（statusCode=0 表示未到达上游），成功时 errMsg 应为空。
func AppendAccountAttempt(c *gin.Context, account *Account, startedAt time.Time, statusCode int, errMsg string, success bool) {
	if c == nil || account == nil {
		return
	}

	attempt := AccountAttempt{
		AtUnixMs:  time.Now().UnixMilli(),
		AccountID: account.ID,
		AccountName: account.Name,
		Platform:  account.Platform,
		StatusCode: statusCode,
		DurationMs: time.Since(startedAt).Milliseconds(),
		Success:   success,
	}
	if !success {
		attempt.Error = truncateString(sanitizeUpstreamErrorMessage(strings.TrimSpace(errMsg)), accountAttemptErrorMaxLen)
	}

	existing := AccountAttemptsFromContext(c)
	// 同账号原地重试：合并为一条（错误取最后一次、耗时累加、保留首次时间）。
	if n := len(existing); n > 0 && existing[n-1].AccountID == attempt.AccountID {
		last := existing[n-1]
		last.DurationMs += attempt.DurationMs
		last.StatusCode = attempt.StatusCode
		last.Error = attempt.Error
		last.Success = attempt.Success
		existing[n-1] = last
		c.Set(accountAttemptsContextKey, existing)
		return
	}

	existing = append(existing, attempt)
	c.Set(accountAttemptsContextKey, existing)
}

// AccountAttemptsFromContext 返回当前请求已累积的账号尝试链（可能为 nil）。
func AccountAttemptsFromContext(c *gin.Context) []AccountAttempt {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(accountAttemptsContextKey); ok {
		if arr, ok := v.([]AccountAttempt); ok {
			return arr
		}
	}
	return nil
}
