package service

import (
	"context"
	"strings"
)

type imageBillingSchedulingTierContextKey struct{}
type imageBillingSchedulingAspectRatioContextKey struct{}
type imageBillingSchedulingQualityContextKey struct{}

func WithImageBillingSchedulingTier(ctx context.Context, tier string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	tier = strings.ToUpper(strings.TrimSpace(tier))
	switch tier {
	case ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K:
		return context.WithValue(ctx, imageBillingSchedulingTierContextKey{}, tier)
	default:
		return ctx
	}
}

func ImageBillingSchedulingTierFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	tier, _ := ctx.Value(imageBillingSchedulingTierContextKey{}).(string)
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case ImageBillingSize1K:
		return ImageBillingSize1K
	case ImageBillingSize2K:
		return ImageBillingSize2K
	case ImageBillingSize4K:
		return ImageBillingSize4K
	default:
		return ""
	}
}

func WithImageBillingSchedulingAspectRatio(ctx context.Context, aspectRatio string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	aspectRatio = NormalizeGeminiImageBillingAspectRatio(aspectRatio)
	if aspectRatio == "" {
		return ctx
	}
	return context.WithValue(ctx, imageBillingSchedulingAspectRatioContextKey{}, aspectRatio)
}

func ImageBillingSchedulingAspectRatioFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	aspectRatio, _ := ctx.Value(imageBillingSchedulingAspectRatioContextKey{}).(string)
	return NormalizeGeminiImageBillingAspectRatio(aspectRatio)
}

func WithImageBillingSchedulingQuality(ctx context.Context, quality string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	quality = NormalizeOpenAIImageQuality(quality)
	if quality == "" {
		return ctx
	}
	return context.WithValue(ctx, imageBillingSchedulingQualityContextKey{}, quality)
}

func ImageBillingSchedulingQualityFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	quality, _ := ctx.Value(imageBillingSchedulingQualityContextKey{}).(string)
	return NormalizeOpenAIImageQuality(quality)
}

type imageBillingSchedulingImageCountContextKey struct{}

// WithImageBillingSchedulingImageCount 记录请求携带的参考图数量(edits 请求),
// 供图片计费路由按数量选择账号。0 表示无参考图。
func WithImageBillingSchedulingImageCount(ctx context.Context, count int) context.Context {
	if ctx == nil || count < 0 {
		return ctx
	}
	// 0(文生图)也是有效值:需与「未注入」区分,以便规则按 0 张参考图精确匹配。
	return context.WithValue(ctx, imageBillingSchedulingImageCountContextKey{}, count)
}

func ImageBillingSchedulingImageCountFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	count, _ := ctx.Value(imageBillingSchedulingImageCountContextKey{}).(int)
	return count
}

type imageBillingErrorFailoverScopeContextKey struct{}

// WithImageBillingErrorFailoverScope 标记本次请求为图片生成请求,
// 使账号级错误自动切换策略(error_failover_rules)在 Gemini 生图链路生效。
// 仅图片请求注入,文本 generateContent 不受策略影响。
func WithImageBillingErrorFailoverScope(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, imageBillingErrorFailoverScopeContextKey{}, true)
}

func ImageBillingErrorFailoverScopeFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(imageBillingErrorFailoverScopeContextKey{}).(bool)
	return enabled
}

// IsGeminiImageGenerationRequest 判定 gemini v1beta 请求是否为图片生成请求:
// 携带参考图、声明了宽高比,或模型名为 gemini 兼容图片模型(含映射键)。
func IsGeminiImageGenerationRequest(info GeminiImageBillingRequestInfo, model string) bool {
	if info.RefImageCount > 0 || info.AspectRatio != "" {
		return true
	}
	return isGeminiCompatibleImageModel(model)
}
