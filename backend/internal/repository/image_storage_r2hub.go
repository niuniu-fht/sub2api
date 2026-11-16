package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// beijingTz 用固定偏移表示北京时间，避免运行环境缺 tzdata（alpine 镜像）时解析失败。
var beijingTz = time.FixedZone("CST", 8*3600)

// InR2HubUploadWindow 判断当前北京时间是否在上传窗口内。
// 窗口格式 "HH:MM-HH:MM"（支持跨午夜，如 20:00-05:00）；空串或非法值按全天处理。
func InR2HubUploadWindow(window string, now time.Time) bool {
	raw := strings.TrimSpace(window)
	if raw == "" {
		return true
	}
	pieces := strings.SplitN(raw, "-", 2)
	if len(pieces) != 2 {
		return true
	}
	start, err1 := parseHHMM(pieces[0])
	end, err2 := parseHHMM(pieces[1])
	if err1 != nil || err2 != nil {
		return true
	}
	current := now.In(beijingTz)
	minutes := current.Hour()*60 + current.Minute()
	if start <= end {
		return minutes >= start && minutes < end
	}
	// 跨午夜窗口
	return minutes >= start || minutes < end
}

// parseHHMM 把 "HH:MM" 解析成当天的分钟数。
func parseHHMM(s string) (int, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid HH:MM value %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, fmt.Errorf("invalid hour in %q", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid minute in %q", s)
	}
	return h*60 + m, nil
}

// R2HubImageStorage 经 r2hub 网关（多账号 Cloudflare R2 聚合服务）上传图片，
// 返回公网直链（pub-*.r2.dev/...）。与 S3ImageStorage 一样实现 service.ImageStorage。
type R2HubImageStorage struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

var _ service.ImageStorage = (*R2HubImageStorage)(nil)

// NewR2HubImageStorage 构造 r2hub 网关客户端；调用方应先确认 cfg.Enabled 且凭证齐全。
func NewR2HubImageStorage(cfg config.ImageStorageR2HubConfig) *R2HubImageStorage {
	timeout := time.Duration(cfg.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &R2HubImageStorage{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		client:  &http.Client{Timeout: timeout},
	}
}

// Save 以 multipart 把图片字节 POST 到 r2hub /upload，解析响应 JSON 的 url 字段返回。
func (s *R2HubImageStorage) Save(ctx context.Context, key, contentType string, data []byte) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	filename := key
	if idx := strings.LastIndexByte(key, '/'); idx >= 0 {
		filename = key[idx+1:]
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("r2hub multipart: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("r2hub multipart write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("r2hub multipart close: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/upload", body)
	if err != nil {
		return "", fmt.Errorf("r2hub build request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("r2hub upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("r2hub read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("r2hub upload: unexpected status %d", resp.StatusCode)
	}
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", fmt.Errorf("r2hub parse response: %w", err)
	}
	url := strings.TrimSpace(payload.URL)
	if url == "" {
		return "", fmt.Errorf("r2hub response missing url field")
	}
	return url, nil
}

// windowedR2HubStorage 装饰器：上传窗口内优先走 r2hub（Cloudflare R2 直链），
// 窗口外或 r2hub 任何失败都回退主存储（MinIO/S3）。装饰器不额外产生错误——
// r2hub 故障不能影响异步生图任务的成败语义。
type windowedR2HubStorage struct {
	primary service.ImageStorage
	r2hub   *R2HubImageStorage
	window  string
	nowFunc func() time.Time // 可注入时钟，测试用
}

var _ service.ImageStorage = (*windowedR2HubStorage)(nil)

// WrapWindowedR2Hub 在主存储外包一层 r2hub 窗口装饰器。
// cfg 未启用或凭证不全时原样返回 primary（记一条 warn，不中断异步生图）。
func WrapWindowedR2Hub(primary service.ImageStorage, cfg config.ImageStorageR2HubConfig) service.ImageStorage {
	if !cfg.Enabled {
		return primary
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" {
		logger.L().Warn("image_storage.r2hub enabled but base_url/api_key incomplete; falling back to primary storage only")
		return primary
	}
	return &windowedR2HubStorage{
		primary: primary,
		r2hub:   NewR2HubImageStorage(cfg),
		window:  cfg.UploadWindow,
		nowFunc: time.Now,
	}
}

// Save 窗口内优先 r2hub，其余情况走主存储；r2hub 失败不报错、直接回退。
func (w *windowedR2HubStorage) Save(ctx context.Context, key, contentType string, data []byte) (string, error) {
	now := w.nowFunc
	if now == nil {
		now = time.Now
	}
	if w.r2hub != nil && InR2HubUploadWindow(w.window, now()) {
		url, err := w.r2hub.Save(ctx, key, contentType, data)
		if err == nil {
			return url, nil
		}
		logger.L().Warn("image_storage.r2hub upload failed; falling back to primary storage",
			zap.String("key", key), zap.Error(err))
	}
	return w.primary.Save(ctx, key, contentType, data)
}
