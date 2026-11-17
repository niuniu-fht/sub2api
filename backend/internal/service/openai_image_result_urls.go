package service

import (
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/gin-gonic/gin"
)

// openAIImageResultURLsContextKey 挂在 gin ctx 上的最终返回图片 URL 快照。
// 与账号尝试链（account_attempt_tracker.go）同一模式：service 在把响应写给
// 客户端前采集，handler 在提交 usage 前快照，随 usage_logs.image_urls 落库。
const openAIImageResultURLsContextKey = "service.openai_image_result_urls"

// maxOpenAIImageResultURLs 单行最多持久化的 URL 条数：默认生图 ≤10 张，
// 超出部分截断，防止极端响应把 JSONB 撑大。
const maxOpenAIImageResultURLs = 8

// CaptureOpenAIFinalImageURLs 从**最终写回客户端的响应体**里提取图片 URL，
// 供管理端核查"下游实际拿到的 URL"（应为图床/r2hub 转存后的地址，而非上游原始地址）。
//
// 口径：
//   - 仅提取 data[].url 中 scheme 为 http/https 的值；data: 内联与纯 b64_json
//     （未开图床转换的返回形态）不采集，列留 NULL；
//   - 去重保序，最多 maxOpenAIImageResultURLs 条；
//   - body 不是合法 JSON 或没有 data 数组时静默跳过（非 images 响应不误采）。
//
// 调用点：OpenAI images 非流式响应的各 c.Data(...) 写回之前（4 处分支收敛形态一致）。
func CaptureOpenAIFinalImageURLs(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	parsed := gjson.GetBytes(body, "data")
	if !parsed.IsArray() {
		return
	}
	seen := make(map[string]struct{}, 4)
	urls := make([]string, 0, 4)
	for _, item := range parsed.Array() {
		raw := strings.TrimSpace(item.Get("url").String())
		if raw == "" || !isHTTPURL(raw) {
			continue
		}
		if _, dup := seen[raw]; dup {
			continue
		}
		seen[raw] = struct{}{}
		urls = append(urls, raw)
		if len(urls) >= maxOpenAIImageResultURLs {
			break
		}
	}
	if len(urls) == 0 {
		return
	}
	c.Set(openAIImageResultURLsContextKey, urls)
}

// FinalImageURLsFromContext 返回本请求已采集的最终返回图片 URL（可能为 nil）。
// handler 在提交 usage 前调用：异步计费 worker 里不得再访问 gin.Context。
func FinalImageURLsFromContext(c *gin.Context) []string {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(openAIImageResultURLsContextKey); ok {
		if urls, ok := v.([]string); ok {
			return urls
		}
	}
	return nil
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
