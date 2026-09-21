package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	responseheaders "github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
)

// 通用异步任务上游适配器:把标准 OpenAI Images 请求转换成
// "创建任务 → 轮询状态 → 拉取结果"的第三方异步生图接口调用,
// 再把结果转回标准 OpenAI Images 响应。上游差异全部由账号
// Extra["openai_image_async_task"] 里的 JSON 配置描述,无需改代码。
//
// 配置示例(FastStable):
// {
//   "base_url": "https://www.faststable.cc",
//   "api_key": "sk-xxx",
//   "headers": {"User-Agent": "Mozilla/5.0"},
//   "create": {
//     "path": "/v2/images/tasks",
//     "body": {
//       "model": "gpt-image-2",
//       "channelId": "gpt-image-default",
//       "params": {"prompt": "{{prompt}}", "count": "{{n}}",
//                  "size": "{{size}}", "quality": "{{quality}}"}
//     },
//     "task_id_path": "task_id"
//   },
//   "status": {
//     "path": "/v2/images/tasks/{{task_id}}",
//     "status_path": "status",
//     "success_value": "succeeded",
//     "failure_value": "failed"
//   },
//   "result": {"image_urls_path": "output.images.#.url"},
//   "error_message_path": "error.message",
//   "poll_interval_seconds": 5,
//   "timeout_seconds": 300
// }

const (
	asyncTaskDefaultPollIntervalSeconds = 5
	asyncTaskDefaultTimeoutSeconds      = 300
	asyncTaskMaxImageBytes              = 25 << 20
	asyncTaskExtraConfigKey             = "openai_image_async_task"
)

type asyncTaskUpstreamConfig struct {
	BaseURL         string            `json:"base_url"`
	APIKey          string            `json:"api_key"`
	Headers         map[string]string `json:"headers"`
	CreateMethod    string            `json:"create_method"`
	CreatePath      string            `json:"create_path"`
	CreateBody      map[string]any    `json:"create_body"`
	CreateTaskIDPath string           `json:"task_id_path"`
	StatusMethod    string            `json:"status_method"`
	StatusPath      string            `json:"status_path"`
	StatusValuePath string            `json:"status_path_value"`
	SuccessValue    string            `json:"success_value"`
	FailureValue    string            `json:"failure_value"`
	ImageURLsPath   string            `json:"image_urls_path"`
	ErrorMessagePath string           `json:"error_message_path"`
	PollIntervalSeconds int           `json:"poll_interval_seconds"`
	TimeoutSeconds  int               `json:"timeout_seconds"`

	PollInterval time.Duration `json:"-"`
	PollTimeout  time.Duration `json:"-"`
}

// getAsyncTaskUpstreamConfig 读取并校验账号的异步上游配置;未配置返回 nil。
func getAsyncTaskUpstreamConfig(account *Account) (*asyncTaskUpstreamConfig, error) {
	if account == nil || !account.IsOpenAI() || len(account.Extra) == 0 {
		return nil, nil
	}
	raw, ok := account.Extra[asyncTaskExtraConfigKey]
	if !ok || raw == nil {
		return nil, nil
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid async task config: %w", err)
	}
	var file asyncTaskUpstreamConfig
	if err := json.Unmarshal(blob, &file); err != nil {
		return nil, fmt.Errorf("invalid async task config: %w", err)
	}
	file.CreateMethod = strings.ToUpper(strings.TrimSpace(file.CreateMethod))
	file.StatusMethod = strings.ToUpper(strings.TrimSpace(file.StatusMethod))
	file.BaseURL = strings.TrimRight(strings.TrimSpace(file.BaseURL), "/")
	file.StatusValuePath = strings.TrimSpace(file.StatusValuePath)
	cfg := &file
	if cfg.BaseURL == "" || cfg.CreatePath == "" || cfg.CreateTaskIDPath == "" ||
		cfg.StatusPath == "" || cfg.StatusValuePath == "" || cfg.SuccessValue == "" ||
		cfg.ImageURLsPath == "" {
		return nil, fmt.Errorf("async task config missing required fields (base_url/create_path/task_id_path/status_path/status_value/success_value/image_urls_path)")
	}
	if cfg.CreateMethod == "" {
		cfg.CreateMethod = http.MethodPost
	}
	if cfg.StatusMethod == "" {
		cfg.StatusMethod = http.MethodGet
	}
	interval := file.PollIntervalSeconds
	if interval <= 0 {
		interval = asyncTaskDefaultPollIntervalSeconds
	}
	cfg.PollInterval = time.Duration(interval) * time.Second
	timeout := file.TimeoutSeconds
	if timeout <= 0 {
		timeout = asyncTaskDefaultTimeoutSeconds
	}
	cfg.PollTimeout = time.Duration(timeout) * time.Second
	return cfg, nil
}

// resolveAsyncTaskTemplate 解析模板:叶子字符串恰为 "{{var}}" 时替换为原始类型值,
// 否则做字符串内插值。未知变量替换为空串。
func resolveAsyncTaskTemplate(node any, vars map[string]any) any {
	switch typed := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = resolveAsyncTaskTemplate(v, vars)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = resolveAsyncTaskTemplate(v, vars)
		}
		return out
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") && strings.Count(trimmed, "{{") == 1 {
			name := strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}")
			name = strings.TrimSpace(name)
			if value, ok := vars[name]; ok {
				return value
			}
			return ""
		}
		for name, value := range vars {
			placeholder := "{{" + name + "}}"
			if strings.Contains(typed, placeholder) {
				typed = strings.ReplaceAll(typed, placeholder, fmt.Sprint(value))
			}
		}
		return typed
	default:
		return node
	}
}

func asyncTaskTemplateVars(parsed *OpenAIImagesRequest, mappedModel string, inputImageURLs []string) map[string]any {
	sizeTier := normalizeOpenAIImageSizeTier(parsed.Size)
	vars := map[string]any{
		"model":            mappedModel,
		"prompt":           parsed.Prompt,
		"n":                parsed.N,
		"size":             parsed.Size,
		"size_tier":        sizeTier,
		"quality":          parsed.Quality,
		"background":       parsed.Background,
		"output_format":    parsed.OutputFormat,
		"moderation":       parsed.Moderation,
		"input_fidelity":   parsed.InputFidelity,
		"style":            parsed.Style,
		"input_image_urls": inputImageURLs,
	}
	return vars
}

func asyncTaskBuildRequest(ctx context.Context, method, rawURL string, headers map[string]string, apiKey string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Body = io.NopCloser(strings.NewReader(string(body)))
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if apiKey != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	return req, nil
}

// forwardOpenAIImagesViaAsyncTask 把标准 OpenAI Images 请求转换为异步任务:
// 创建 → 轮询 → 拉取结果 URL,并按 response_format 回写标准响应。
func (s *OpenAIGatewayService) forwardOpenAIImagesViaAsyncTask(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
	cfg *asyncTaskUpstreamConfig,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	upstreamModel := account.GetMappedModel(requestModel)
	if upstreamModel == "" {
		upstreamModel = requestModel
	}
	SetOpsUpstreamModel(c, upstreamModel)

	// 客户端断开不应中断已提交任务的轮询:上游侧任务照样计费,需拿到结果入账。
	pollCtx, cancelPoll := context.WithTimeout(context.WithoutCancel(ctx), cfg.PollTimeout)
	defer cancelPoll()

	// ---- 1. 创建任务 ----
	inputURLs := collectOpenAIImagesInputURLs(parsed, body)
	createBody, err := json.Marshal(resolveAsyncTaskTemplate(cfg.CreateBody, asyncTaskTemplateVars(parsed, upstreamModel, inputURLs)))
	if err != nil {
		return nil, fmt.Errorf("async task create body: %w", err)
	}
	createReq, err := asyncTaskBuildRequest(pollCtx, cfg.CreateMethod, cfg.BaseURL+cfg.CreatePath, cfg.Headers, cfg.APIKey, createBody)
	if err != nil {
		return nil, fmt.Errorf("async task create request: %w", err)
	}
	resp, err := s.httpUpstream.Do(createReq, "", account.ID, account.Concurrency)
	if err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(err.Error())}
	}
	createBodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, classifyAsyncTaskUpstreamError(resp.StatusCode, createBodyBytes, cfg)
	}
	taskID := strings.TrimSpace(gjson.GetBytes(createBodyBytes, cfg.CreateTaskIDPath).String())
	if taskID == "" {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: createBodyBytes}
	}

	// ---- 2. 轮询状态 ----
	statusPath := strings.ReplaceAll(cfg.StatusPath, "{{task_id}}", taskID)
	var lastBody []byte
	for {
		select {
		case <-pollCtx.Done():
			return nil, &UpstreamFailoverError{StatusCode: http.StatusGatewayTimeout, ResponseBody: []byte("async task polling timed out: " + taskID)}
		default:
		}
		time.Sleep(cfg.PollInterval)
		pollReq, err := asyncTaskBuildRequest(pollCtx, cfg.StatusMethod, cfg.BaseURL+statusPath, cfg.Headers, cfg.APIKey, nil)
		if err != nil {
			return nil, fmt.Errorf("async task status request: %w", err)
		}
		pollResp, err := s.httpUpstream.Do(pollReq, "", account.ID, account.Concurrency)
		if err != nil {
			// 轮询中的瞬时网络错误:继续到下一轮,不吃掉整个任务。
			continue
		}
		lastBody, _ = io.ReadAll(io.LimitReader(pollResp.Body, 1<<20))
		_ = pollResp.Body.Close()
		if pollResp.StatusCode >= 400 {
			return nil, classifyAsyncTaskUpstreamError(pollResp.StatusCode, lastBody, cfg)
		}
		status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(lastBody, cfg.StatusValuePath).String()))
		switch {
		case status != "" && status == strings.ToLower(cfg.SuccessValue):
			return s.finishOpenAIImagesAsyncTask(ctx, c, account, parsed, upstreamModel, cfg, lastBody, startTime, resp)
		case status != "" && cfg.FailureValue != "" && status == strings.ToLower(cfg.FailureValue):
			return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: lastBody}
		default:
			// pending/processing/未知:继续轮询
		}
	}
}

func classifyAsyncTaskUpstreamError(statusCode int, body []byte, cfg *asyncTaskUpstreamConfig) error {
	message := ""
	if cfg != nil && cfg.ErrorMessagePath != "" {
		message = strings.TrimSpace(gjson.GetBytes(body, cfg.ErrorMessagePath).String())
	}
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if len(message) > 500 {
		message = message[:500]
	}
	if statusCode == http.StatusBadRequest {
		return &OpenAIImagesUpstreamError{StatusCode: statusCode, Message: message}
	}
	return &UpstreamFailoverError{StatusCode: statusCode, ResponseBody: []byte(message)}
}

// finishOpenAIImagesAsyncTask 提取结果 URL,按 response_format 回写标准响应并构造计费结果。
func (s *OpenAIGatewayService) finishOpenAIImagesAsyncTask(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	upstreamModel string,
	cfg *asyncTaskUpstreamConfig,
	statusBody []byte,
	startTime time.Time,
	createResp *http.Response,
) (*OpenAIForwardResult, error) {
	imageURLs := []string{}
	for _, item := range gjson.GetBytes(statusBody, cfg.ImageURLsPath).Array() {
		if u := strings.TrimSpace(item.String()); u != "" {
			imageURLs = append(imageURLs, u)
		}
	}
	if len(imageURLs) == 0 {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte("async task succeeded but no image urls found")}
	}

	responseFormat := strings.ToLower(strings.TrimSpace(parsed.ResponseFormat))
	if responseFormat == "" {
		responseFormat = "b64_json"
	}
	out := []byte(`{"created":0,"data":[]}`)
	out, _ = sjson.SetBytes(out, "created", time.Now().Unix())
	for _, imageURL := range imageURLs {
		item := []byte(`{}`)
		if responseFormat == "url" {
			item, _ = sjson.SetBytes(item, "url", imageURL)
		} else {
			payload, err := downloadAsyncTaskImage(ctx, imageURL)
			if err != nil {
				return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte("async task image download failed: " + err.Error())}
			}
			item, _ = sjson.SetBytes(item, "b64_json", base64.StdEncoding.EncodeToString(payload))
		}
		if parsed.Size != "" {
			item, _ = sjson.SetBytes(item, "size", parsed.Size)
		}
		if parsed.Quality != "" {
			item, _ = sjson.SetBytes(item, "quality", parsed.Quality)
		}
		out, _ = sjson.SetRawBytes(out, "data.-1", item)
	}
	if parsed.OutputFormat != "" {
		out, _ = sjson.SetBytes(out, "output_format", parsed.OutputFormat)
	}
	if parsed.Quality != "" {
		out, _ = sjson.SetBytes(out, "quality", parsed.Quality)
	}
	if parsed.Size != "" {
		out, _ = sjson.SetBytes(out, "size", parsed.Size)
	}
	out, _ = sjson.SetBytes(out, "model", upstreamModel)

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), createResp.Header, s.responseHeaderFilter)
	c.Data(http.StatusOK, "application/json; charset=utf-8", out)

	return &OpenAIForwardResult{
		Model:            strings.TrimSpace(parsed.Model),
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: cfg.CreatePath,
		ResponseHeaders:  createResp.Header.Clone(),
		Duration:         time.Since(startTime),
		Stream:           false,
		ImageCount:       len(imageURLs),
		ImageSize:        parsed.Size,
		ImageQuality:     parsed.Quality,
	}, nil
}

func downloadAsyncTaskImage(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, asyncTaskMaxImageBytes))
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty image payload")
	}
	return payload, nil
}

// collectOpenAIImagesInputURLs 从 JSON edits 请求里提取参考图 URL,供异步上游使用。
func collectOpenAIImagesInputURLs(parsed *OpenAIImagesRequest, body []byte) []string {
	// multipart 上传的参考图是文件字节,无法直接转成 URL;异步上游的参考图
	// 通过 JSON 请求体的 input_urls 数组传入。
	urls := []string{}
	if len(urls) == 0 && gjson.ValidBytes(body) {
		gjson.GetBytes(body, "input_urls").ForEach(func(_, value gjson.Result) bool {
			if u := strings.TrimSpace(value.String()); u != "" {
				urls = append(urls, u)
			}
			return true
		})
	}
	return urls
}
