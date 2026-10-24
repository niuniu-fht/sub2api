package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/tidwall/gjson"
)

type GeminiImageBillingRequestInfo struct {
	Tier        string
	AspectRatio string
	// RefImageCount 请求携带的参考图数量(contents 中内联图片/文件引用的部件数)。
	RefImageCount int
}

func ParseGeminiImageBillingRequestInfo(body []byte) GeminiImageBillingRequestInfo {
	var req antigravity.GeminiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return GeminiImageBillingRequestInfo{Tier: ImageBillingSize1K, RefImageCount: countGeminiRequestReferenceImages(body)}
	}
	info := GeminiImageBillingRequestInfo{Tier: ImageBillingSize1K}
	if req.GenerationConfig != nil && req.GenerationConfig.ImageConfig != nil {
		info.Tier = normalizeOpenAIImageSizeTier(strings.TrimSpace(req.GenerationConfig.ImageConfig.ImageSize))
		info.AspectRatio = NormalizeGeminiImageBillingAspectRatio(req.GenerationConfig.ImageConfig.AspectRatio)
	}
	// 用 gjson 兜底计数参考图:兼容 camelCase 官方拼写与 snake_case 中转拼写,
	// 也覆盖结构体未建模的 fileData 文件引用。
	info.RefImageCount = countGeminiRequestReferenceImages(body)
	return info
}

// countGeminiRequestReferenceImages 统计 contents 各部件中的参考图数量
// (inlineData/inline_data 内联图或 fileData/file_data 文件引用)。
func countGeminiRequestReferenceImages(body []byte) int {
	count := 0
	gjson.GetBytes(body, "contents").ForEach(func(_, content gjson.Result) bool {
		content.Get("parts").ForEach(func(_, part gjson.Result) bool {
			if part.Get("inlineData").Exists() || part.Get("inline_data").Exists() ||
				part.Get("fileData").Exists() || part.Get("file_data").Exists() {
				count++
			}
			return true
		})
		return true
	})
	return count
}
