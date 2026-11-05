package service

import "context"

// geminiImageErrorFailoverError Gemini 生图链路的账号级错误自动切换策略:
// 请求带图片 scope(WithImageBillingErrorFailoverScope 注入)且账号规则
// (error_failover_enabled / error_failover_rules)命中时,返回带 SkipAccountPenalty
// 的 failover 错误——由 handler 切换到路由名单的下一个账号,且不对该账号做任何
// 惩罚标记(不标记状态/不临时封禁/不计健康熔断/不上报调度失败);未命中返回 nil。
func geminiImageErrorFailoverError(ctx context.Context, account *Account, statusCode int, respBody []byte) *UpstreamFailoverError {
	if !ImageBillingErrorFailoverScopeFromContext(ctx) {
		return nil
	}
	if !account.MatchErrorFailoverRules(statusCode, respBody) {
		return nil
	}
	return &UpstreamFailoverError{
		StatusCode:         statusCode,
		ResponseBody:       respBody,
		NextAccountAction:  NextAccountRetry,
		SkipAccountPenalty: true,
	}
}
