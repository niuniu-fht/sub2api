package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// OpenAI Images 协议 → Gemini generateContent 协议转换(账号级开关)。
//
// 下游以 /v1/images/generations|edits + gemini-*-image 模型调用时,网关把请求转成
// Gemini generateContent 发给开了开关的 Gemini API-key 账号,再把返回的
// inlineData 包装回 OpenAI data[].b64_json 响应;账号开启 images_b64_to_url 时
// 自动复用图床转存,下游直接拿到存储 URL。
const AccountExtraGeminiImageBridge = "gemini_image_bridge"

func geminiImageBridgeEnabled(account *Account) bool {
	return account != nil &&
		account.Platform == PlatformGemini &&
		account.Type == AccountTypeAPIKey &&
		account.getExtraBool(AccountExtraGeminiImageBridge)
}

// geminiImageBridgeAccountEligible 桥接账号参与 OpenAI Images 调度的门槛:
// 开关打开、可调度、在请求分组内且配置了 API key。
func (s *OpenAIGatewayService) geminiImageBridgeAccountEligible(ctx context.Context, account *Account, groupID *int64) bool {
	if !geminiImageBridgeEnabled(account) || !account.IsSchedulable() {
		return false
	}
	if strings.TrimSpace(account.GetCredential("api_key")) == "" {
		return false
	}
	if groupID != nil && *groupID > 0 && !s.openAIAccountMatchesSchedulingGroup(account, groupID) {
		return false
	}
	return true
}

type geminiImageBridgeContent struct {
	Role  string                  `json:"role"`
	Parts []geminiImageBridgePart `json:"parts"`
}

type geminiImageBridgePart struct {
	Text       string                       `json:"text,omitempty"`
	InlineData *geminiImageBridgeInlineData `json:"inlineData,omitempty"`
}

type geminiImageBridgeInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiImageBridgeGenerationConfig struct {
	ResponseModalities []string                      `json:"responseModalities"`
	ImageConfig        *geminiImageBridgeImageConfig `json:"imageConfig,omitempty"`
}

type geminiImageBridgeImageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize,omitempty"`
}

type geminiImageBridgePayload struct {
	Contents         []geminiImageBridgeContent         `json:"contents"`
	GenerationConfig *geminiImageBridgeGenerationConfig `json:"generationConfig,omitempty"`
}

// geminiSupportedImageAspectRatios 是 Gemini imageConfig.aspectRatio 接受的比值集合;
// OpenAI size 折算出的比例不在集合内时不带 aspectRatio,交给模型默认 1:1。
var geminiSupportedImageAspectRatios = map[string]struct{}{
	"1:1": {}, "2:3": {}, "3:2": {}, "3:4": {}, "4:3": {}, "4:5": {}, "5:4": {},
	"9:16": {}, "16:9": {}, "21:9": {},
}

// geminiImageAspectRatioFromSize 把 OpenAI 的 "WxH" 尺寸折算成 Gemini 支持的
// aspectRatio;无法折算或比例不受支持时返回空串。
func geminiImageAspectRatioFromSize(size string) string {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(size)), "x", 2)
	if len(parts) != 2 {
		return ""
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return ""
	}
	a, b := w, h
	for b != 0 {
		a, b = b, a%b
	}
	if a <= 0 {
		return ""
	}
	ratio := fmt.Sprintf("%d:%d", w/a, h/a)
	if _, ok := geminiSupportedImageAspectRatios[ratio]; !ok {
		return ""
	}
	return ratio
}

// buildGeminiImageBridgePayload 把 OpenAI Images 请求转成 generateContent 载荷。
// 返回载荷字节与已内联的参考图张数(供日志与观测)。
func buildGeminiImageBridgePayload(
	ctx context.Context,
	s *OpenAIGatewayService,
	account *Account,
	parsed *OpenAIImagesRequest,
) ([]byte, int, error) {
	parts := make([]geminiImageBridgePart, 0, len(parsed.Uploads)+len(parsed.InputImageURLs)+1)
	if prompt := strings.TrimSpace(parsed.Prompt); prompt != "" {
		parts = append(parts, geminiImageBridgePart{Text: prompt})
	}
	inlineCount := 0
	for _, upload := range parsed.Uploads {
		if len(upload.Data) == 0 {
			continue
		}
		mimeType := strings.TrimSpace(upload.ContentType)
		if mimeType == "" {
			mimeType = sniffImageContentType(upload.Data)
		}
		parts = append(parts, geminiImageBridgePart{InlineData: &geminiImageBridgeInlineData{
			MimeType: mimeType,
			Data:     base64.StdEncoding.EncodeToString(upload.Data),
		}})
		inlineCount++
	}
	for _, rawURL := range parsed.InputImageURLs {
		// 复用回填管线的下载(私网拦截/代理),无大小上限,与 edits 参考图转换一致。
		encoded, err := s.fetchOpenAIImageURLBase64(ctx, account, rawURL)
		if err != nil {
			return nil, inlineCount, fmt.Errorf("download reference image failed: %s", err.Error())
		}
		payload, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, inlineCount, fmt.Errorf("decode reference image failed: %s", err.Error())
		}
		parts = append(parts, geminiImageBridgePart{InlineData: &geminiImageBridgeInlineData{
			MimeType: sniffImageContentType(payload),
			Data:     base64.StdEncoding.EncodeToString(payload),
		}})
		inlineCount++
	}
	if len(parts) == 0 {
		return nil, 0, fmt.Errorf("prompt and reference images are both empty")
	}

	payload := geminiImageBridgePayload{
		Contents: []geminiImageBridgeContent{{Role: "user", Parts: parts}},
		GenerationConfig: &geminiImageBridgeGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
		},
	}
	imageConfig := &geminiImageBridgeImageConfig{}
	if ratio := geminiImageAspectRatioFromSize(parsed.Size); ratio != "" {
		imageConfig.AspectRatio = ratio
	}
	if tier := strings.ToUpper(strings.TrimSpace(parsed.SizeTier)); tier != "" {
		imageConfig.ImageSize = tier
	}
	if imageConfig.AspectRatio != "" || imageConfig.ImageSize != "" {
		payload.GenerationConfig.ImageConfig = imageConfig
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, inlineCount, err
	}
	return body, inlineCount, nil
}

type geminiImageBridgeImage struct {
	Base64   string
	MimeType string
}

// parseGeminiImageBridgeResponse 从 generateContent 响应里取图片与用量;
// 无图但带 blockReason/SAFETY 类 finishReason 时返回内容拦截错误。
// 兼容 camelCase(Google 官方)与 snake_case(部分中转)两种字段拼写。
func parseGeminiImageBridgeResponse(raw []byte) ([]geminiImageBridgeImage, OpenAIUsage, int, string, error) {
	images, usage, status, message, err := parseGeminiImageBridgeResponseTyped(raw)
	// camelCase 解析 valid 但无图且字段是 snake_case 拼写时,归一化后重解析一次。
	if err != nil || len(images) > 0 || !isSnakeCaseGeminiResponse(raw) {
		return images, usage, status, message, err
	}
	return parseGeminiImageBridgeResponseTyped(normalizeSnakeCaseGeminiResponse(raw))
}

// isSnakeCaseGeminiResponse 判断响应是否使用 snake_case 字段拼写。
func isSnakeCaseGeminiResponse(raw []byte) bool {
	if gjson.GetBytes(raw, "inlineData").Exists() || gjson.GetBytes(raw, "promptFeedback").Exists() ||
		gjson.GetBytes(raw, "usageMetadata").Exists() {
		return false
	}
	return gjson.GetBytes(raw, "prompt_feedback").Exists() || gjson.GetBytes(raw, "usage_metadata").Exists() ||
		gjson.GetBytes(raw, "finish_reason").Exists()
}

// normalizeSnakeCaseGeminiResponse 把 snake_case 拼写转成官方 camelCase,供统一解析。
func normalizeSnakeCaseGeminiResponse(raw []byte) []byte {
	out := raw
	replacements := [][2]string{
		{"prompt_feedback", "promptFeedback"},
		{"block_reason", "blockReason"},
		{"finish_reason", "finishReason"},
		{"usage_metadata", "usageMetadata"},
		{"prompt_token_count", "promptTokenCount"},
		{"candidates_token_count", "candidatesTokenCount"},
		{"total_token_count", "totalTokenCount"},
		{"thoughts_token_count", "thoughtsTokenCount"},
		{"inline_data", "inlineData"},
		{"mime_type", "mimeType"},
	}
	for _, pair := range replacements {
		out = bytes.ReplaceAll(out, []byte(`"`+pair[0]+`"`), []byte(`"`+pair[1]+`"`))
	}
	return out
}

// parseGeminiImageBridgeResponseTyped 按官方 camelCase 字段解析响应。
func parseGeminiImageBridgeResponseTyped(raw []byte) ([]geminiImageBridgeImage, OpenAIUsage, int, string, error) {
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback *struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
			ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, OpenAIUsage{}, 0, "", fmt.Errorf("invalid gemini response: %s", err.Error())
	}

	usage := OpenAIUsage{
		InputTokens:  parsed.UsageMetadata.PromptTokenCount,
		OutputTokens: parsed.UsageMetadata.CandidatesTokenCount + parsed.UsageMetadata.ThoughtsTokenCount,
	}

	images := make([]geminiImageBridgeImage, 0, 2)
	for _, candidate := range parsed.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData == nil || strings.TrimSpace(part.InlineData.Data) == "" {
				continue
			}
			mimeType := strings.TrimSpace(part.InlineData.MimeType)
			if mimeType != "" && !strings.HasPrefix(mimeType, "image/") {
				continue
			}
			images = append(images, geminiImageBridgeImage{Base64: part.InlineData.Data, MimeType: mimeType})
		}
	}
	if len(images) > 0 {
		return images, usage, 0, "", nil
	}

	message := ""
	if parsed.PromptFeedback != nil && strings.TrimSpace(parsed.PromptFeedback.BlockReason) != "" {
		message = fmt.Sprintf("content blocked by upstream moderation: %s", strings.TrimSpace(parsed.PromptFeedback.BlockReason))
	} else {
		for _, candidate := range parsed.Candidates {
			reason := strings.TrimSpace(candidate.FinishReason)
			if reason == "" || strings.EqualFold(reason, "STOP") {
				continue
			}
			message = fmt.Sprintf("gemini returned no image, finishReason=%s", reason)
			break
		}
		if message == "" {
			message = "gemini returned no image data"
		}
	}
	return nil, usage, http.StatusBadRequest, message, nil
}

// geminiImageBridgeStatusRetryable 判定哪些上游状态码应触发账号切换(轮询/容错),
// 其余 4xx(含内容审核 400)按用户错误原样返回下游。
func geminiImageBridgeStatusRetryable(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// defaultGeminiBridgeModel 客户端模型名解析不出 gemini 图片模型时的桥接默认模型。
const defaultGeminiBridgeModel = "gemini-2.5-flash-image"

// resolveGeminiBridgeUpstreamModel 解析桥接上游模型:
//  1. 账号模型映射命中且为 gemini 图片模型 → 用之;
//  2. 客户端名剥掉 gpt-image- 前缀后是 gemini 图片模型 → 用之
//     (gpt-image-gemini-3.1-flash-image → gemini-3.1-flash-image);
//  3. 否则回落默认模型,让纯桥接分组的任意 OpenAI 调用直接可用。
func resolveGeminiBridgeUpstreamModel(account *Account, requestModel string) string {
	if mapped := strings.TrimSpace(account.GetMappedModel(requestModel)); isGeminiCompatibleImageModel(mapped) {
		return mapped
	}
	trimmed := strings.TrimSpace(requestModel)
	if len(trimmed) > len("gpt-image-") && strings.EqualFold(trimmed[:len("gpt-image-")], "gpt-image-") {
		stripped := trimmed[len("gpt-image-"):]
		if isGeminiCompatibleImageModel(stripped) {
			return stripped
		}
	}
	return defaultGeminiBridgeModel
}

// forwardOpenAIImagesViaGemini 把 OpenAI Images 请求经协议转换发给 Gemini 账号,
// 并把响应包装回 OpenAI 格式(含图床 URL 改写、错误透传、计费用量)。
func (s *OpenAIGatewayService) forwardOpenAIImagesViaGemini(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	clientModel := strings.TrimSpace(parsed.Model)
	requestModel := clientModel
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	upstreamModel := resolveGeminiBridgeUpstreamModel(account, requestModel)
	SetOpsUpstreamModel(c, upstreamModel)
	if parsed.Stream {
		return nil, &OpenAIImagesUpstreamError{
			StatusCode: http.StatusBadRequest,
			ErrorType:  "invalid_request_error",
			Message:    "streaming is not supported for gemini image bridge accounts",
		}
	}

	n := parsed.N
	if n <= 0 {
		n = 1
	}

	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	upstreamURL, err := buildGeminiAIStudioModelActionURL(baseURL, upstreamModel, "generateContent", false)
	if err != nil {
		return nil, &OpenAIImagesUpstreamError{StatusCode: http.StatusInternalServerError, Message: err.Error()}
	}

	payloadBody, inlineRefCount, err := buildGeminiImageBridgePayload(ctx, s, account, parsed)
	if err != nil {
		return nil, &OpenAIImagesUpstreamError{
			StatusCode: http.StatusBadRequest,
			ErrorType:  "invalid_request_error",
			Message:    err.Error(),
		}
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images gemini bridge request_model=%s upstream_model=%s account_type=%s ref_images=%d count=%d",
		clientModel,
		upstreamModel,
		account.Type,
		inlineRefCount,
		n,
	)

	// 生图长耗时且上游已产生成本:与 OpenAI 直连路径一致,客户端断开不取消上游。
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	images := make([]geminiImageBridgeImage, 0, n)
	usage := OpenAIUsage{}
	for i := 0; i < n; i++ {
		callImages, callUsage, err := s.callGeminiImageBridgeOnce(upstreamCtx, c, account, upstreamURL, payloadBody)
		usage.InputTokens += callUsage.InputTokens
		usage.OutputTokens += callUsage.OutputTokens
		if err != nil {
			return nil, err
		}
		images = append(images, callImages...)
	}

	items := make([]map[string]any, 0, len(images))
	for _, image := range images {
		item := map[string]any{"b64_json": image.Base64}
		if ext := geminiImageFormatFromMimeType(image.MimeType); ext != "" {
			item["output_format"] = ext
		}
		items = append(items, item)
	}
	responsePayload := map[string]any{
		"created": time.Now().Unix(),
		"data":    items,
		"usage": map[string]any{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
			"total_tokens":  usage.InputTokens + usage.OutputTokens,
		},
	}
	responseBody, err := json.Marshal(responsePayload)
	if err != nil {
		return nil, &OpenAIImagesUpstreamError{StatusCode: http.StatusInternalServerError, Message: err.Error()}
	}
	responseBody = s.rewriteOpenAIImagesB64ToURL(ctx, account, parsed, responseBody)

	c.Data(http.StatusOK, "application/json", responseBody)
	firstToken := int(time.Since(startTime).Milliseconds())
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, int64(firstToken))
	return &OpenAIForwardResult{
		Model:            clientModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: "/v1beta/models/" + upstreamModel + ":generateContent",
		Usage:            usage,
		ImageCount:       len(items),
		ImageSize:        strings.TrimSpace(parsed.Size),
		ImageQuality:     strings.TrimSpace(parsed.Quality),
		Stream:           false,
		Duration:         time.Since(startTime),
		FirstTokenMs:     &firstToken,
	}, nil
}

// callGeminiImageBridgeOnce 发一次 generateContent 并分类错误:
// 透传命中→原样返回;可重试状态→failover 切换账号;其余→用户错误直返。
func (s *OpenAIGatewayService) callGeminiImageBridgeOnce(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	upstreamURL string,
	payload []byte,
) ([]geminiImageBridgeImage, OpenAIUsage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(payload))
	if err != nil {
		return nil, OpenAIUsage{}, &OpenAIImagesUpstreamError{StatusCode: http.StatusInternalServerError, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", account.GetCredential("api_key"))

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		return nil, OpenAIUsage{}, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		// 错误透传规则:命中即按规则原样返回,不再切换账号。
		if ptStatus, _, ptErrMsg, ptMatched := applyErrorPassthroughRule(c, PlatformGemini, resp.StatusCode, respBody, resp.StatusCode, "upstream_error", upstreamMsg); ptMatched {
			upErr := &OpenAIImagesUpstreamError{StatusCode: ptStatus, ErrorType: "upstream_error", Message: ptErrMsg}
			writeOpenAIImagesUpstreamErrorResponse(c, upErr)
			return nil, OpenAIUsage{}, upErr
		}
		if geminiImageBridgeStatusRetryable(resp.StatusCode) {
			return nil, OpenAIUsage{}, &UpstreamFailoverError{
				StatusCode:        resp.StatusCode,
				ResponseHeaders:   resp.Header.Clone(),
				ResponseBody:      respBody,
				NextAccountAction: NextAccountRetry,
			}
		}
		upErr := &OpenAIImagesUpstreamError{StatusCode: resp.StatusCode, ErrorType: "upstream_error", Message: upstreamMsg}
		writeOpenAIImagesUpstreamErrorResponse(c, upErr)
		return nil, OpenAIUsage{}, upErr
	}

	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	_ = resp.Body.Close()
	if err != nil {
		return nil, OpenAIUsage{}, err
	}
	images, usage, status, message, err := parseGeminiImageBridgeResponse(body)
	if err != nil {
		return nil, usage, &OpenAIImagesUpstreamError{StatusCode: http.StatusBadGateway, ErrorType: "upstream_error", Message: err.Error()}
	}
	if len(images) == 0 {
		// 内容拦截:200 但无图。按 400+原始响应体过一遍透传规则,命中则原样返回。
		if ptStatus, _, ptErrMsg, ptMatched := applyErrorPassthroughRule(c, PlatformGemini, status, body, status, "upstream_error", message); ptMatched {
			upErr := &OpenAIImagesUpstreamError{StatusCode: ptStatus, ErrorType: "upstream_error", Message: ptErrMsg}
			writeOpenAIImagesUpstreamErrorResponse(c, upErr)
			return nil, usage, upErr
		}
		upErr := &OpenAIImagesUpstreamError{StatusCode: status, ErrorType: "upstream_error", Message: message}
		writeOpenAIImagesUpstreamErrorResponse(c, upErr)
		return nil, usage, upErr
	}
	return images, usage, nil
}

func geminiImageFormatFromMimeType(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		return ""
	}
}
