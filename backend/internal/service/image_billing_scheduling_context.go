package service

import (
	"context"
	"strings"
)

type imageBillingSchedulingTierContextKey struct{}

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
