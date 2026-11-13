package service

import (
	"context"
	"net/http"
)

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

// shouldFailoverGeminiImageUpstreamStatus Gemini 图片生成请求的换号判定:
// 只有 400 不换号——请求本身的问题,换其他账号也无法解决;其余状态码
// (404 model_not_found / 408 / 429 / 5xx 等)一律换路由链下一个账号,
// 与 OpenAI Images 路径同语义,保证图片计费路由链的容错真正生效。
// 仅对带图片 scope 的请求生效;非图片请求维持原白名单语义。
// 账号状态标记(429 限流冷却/鉴权封禁等)仍由各调用方在换号判定前完成,不受此影响。
func shouldFailoverGeminiImageUpstreamStatus(ctx context.Context, statusCode int) bool {
	if statusCode == http.StatusBadRequest {
		return false
	}
	return ImageBillingErrorFailoverScopeFromContext(ctx)
}
