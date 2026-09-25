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
	if ctx == nil || count <= 0 {
		return ctx
	}
	return context.WithValue(ctx, imageBillingSchedulingImageCountContextKey{}, count)
}

func ImageBillingSchedulingImageCountFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	count, _ := ctx.Value(imageBillingSchedulingImageCountContextKey{}).(int)
	return count
}
