package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

type GeminiImageBillingRequestInfo struct {
	Tier        string
	AspectRatio string
}

func ParseGeminiImageBillingRequestInfo(body []byte) GeminiImageBillingRequestInfo {
	var req antigravity.GeminiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return GeminiImageBillingRequestInfo{Tier: ImageBillingSize1K}
	}
	if req.GenerationConfig == nil || req.GenerationConfig.ImageConfig == nil {
		return GeminiImageBillingRequestInfo{Tier: ImageBillingSize1K}
	}
	return GeminiImageBillingRequestInfo{
		Tier:        normalizeOpenAIImageSizeTier(strings.TrimSpace(req.GenerationConfig.ImageConfig.ImageSize)),
		AspectRatio: NormalizeGeminiImageBillingAspectRatio(req.GenerationConfig.ImageConfig.AspectRatio),
	}
}
