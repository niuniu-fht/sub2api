package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tidwall/gjson"
	"mime/multipart"
)

// JSON 形式的 edits 参考(image_url)→ Multipart 文件上传 的内联转换:
// 用户以 JSON 调用,但目标号池只收文件上传时,网关把参考图下载后
// 直接转成 multipart 文件字段转发,无需对象存储中转。

const (
	editsMultipartExtraKey   = "openai_images_edits_multipart_upload"
	editsImageMaxBytes       = 25 << 20 // 单张参考图上限
	editsImageFetchTimeout   = 30 * time.Second
	editsImageMaxConcurrent  = 4
	editsMultipartTotalLimit = 64 << 20
)

func accountWantsEditsMultipartUpload(account *Account) bool {
	if account == nil || len(account.Extra) == 0 {
		return false
	}
	flag, ok := account.Extra[editsMultipartExtraKey].(bool)
	return ok && flag
}

// editsImageFetchClient 参考图下载客户端:URL 来自终端用户,
// DialContext Control 禁止解析到内网/环网段,防 SSRF。
// editsImagePrivateIPGuard 默认拦截内网/环网段;测试可临时替换。
var editsImagePrivateIPGuard = isPrivateIP

var editsImageFetchClient = &http.Client{
	Timeout: editsImageFetchTimeout,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConnsPerHost:   editsImageMaxConcurrent,
		ResponseHeaderTimeout: editsImageFetchTimeout,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				if editsImagePrivateIPGuard(net.ParseIP(host)) {
					return fmt.Errorf("blocked private address: %s", host)
				}
				return nil
			},
		}).DialContext,
		TLSHandshakeTimeout: 15 * time.Second,
	},
}

// fetchEditsImage 下载参考图字节;data:URL 本地解码,http(s) 走带 SSRF 防护的客户端。
func fetchEditsImage(ctx context.Context, rawURL string) ([]byte, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, "", fmt.Errorf("empty image url")
	}
	if strings.HasPrefix(rawURL, "data:") {
		comma := strings.Index(rawURL, ",")
		if comma == -1 {
			return nil, "", fmt.Errorf("invalid data url")
		}
		contentType := strings.TrimPrefix(rawURL[:comma], "data:")
		if semi := strings.Index(contentType, ";"); semi != -1 {
			contentType = contentType[:semi]
		}
		payload, err := base64.StdEncoding.DecodeString(rawURL[comma+1:])
		if err != nil {
			return nil, "", fmt.Errorf("decode data url: %w", err)
		}
		return payload, contentType, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := editsImageFetchClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, editsImageMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(payload) == 0 {
		return nil, "", fmt.Errorf("empty image payload")
	}
	if len(payload) > editsImageMaxBytes {
		return nil, "", fmt.Errorf("image exceeds %d bytes", editsImageMaxBytes)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || strings.EqualFold(contentType, "application/octet-stream") {
		contentType = detectEditsImageContentType(payload)
	}
	return payload, contentType, nil
}

func detectEditsImageContentType(payload []byte) string {
	switch {
	case len(payload) > 3 && payload[0] == 0x89 && payload[1] == 'P' && payload[2] == 'N' && payload[3] == 'G':
		return "image/png"
	case len(payload) > 2 && payload[0] == 0xFF && payload[1] == 0xD8:
		return "image/jpeg"
	case len(payload) > 11 && string(payload[:4]) == "RIFF" && string(payload[8:12]) == "WEBP":
		return "image/webp"
	default:
		return "image/png"
	}
}

func imageFileExtension(contentType string) string {
	if extensions, err := mime.ExtensionsByType(strings.TrimSpace(contentType)); err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	return ".png"
}

// convertOpenAIImagesJSONEditsToMultipart 把 JSON edits 请求体转成 multipart:
// 参考图并行下载(上限 editsImageMaxConcurrent)后作为 image 文件字段,
// 其余顶层标量字段转为文本字段;model/images/mask 由后续逻辑处理。
func convertOpenAIImagesJSONEditsToMultipart(ctx context.Context, body []byte, parsed *OpenAIImagesRequest) ([]byte, string, error) {
	if len(parsed.InputImageURLs) == 0 {
		return nil, "", fmt.Errorf("no reference image urls to convert")
	}

	imagePayloads := make([][]byte, len(parsed.InputImageURLs))
	imageTypes := make([]string, len(parsed.InputImageURLs))
	failures := make([]error, len(parsed.InputImageURLs))

	sem := make(chan struct{}, editsImageMaxConcurrent)
	var wg sync.WaitGroup
	for i, rawURL := range parsed.InputImageURLs {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, url string) {
			defer wg.Done()
			defer func() { <-sem }()
			payload, contentType, err := fetchEditsImage(ctx, url)
			imagePayloads[idx] = payload
			imageTypes[idx] = contentType
			failures[idx] = err
		}(i, rawURL)
	}
	wg.Wait()
	for i, err := range failures {
		if err != nil {
			return nil, "", fmt.Errorf("reference image %d download failed: %w", i+1, err)
		}
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	// 顶层标量字段 → 文本字段(model/images/mask/stream 除外,由后续逻辑处理)。
	pristine := gjson.ParseBytes(body)
	pristine.ForEach(func(key, value gjson.Result) bool {
		name := key.String()
		if name == "" || name == "model" || name == "images" || name == "mask" || name == "stream" {
			return true
		}
		if value.IsArray() || value.IsObject() {
			return true
		}
		_ = writer.WriteField(name, value.String())
		return true
	})

	total := 0
	for i, payload := range imagePayloads {
		total += len(payload)
		if total > editsMultipartTotalLimit {
			return nil, "", fmt.Errorf("reference images exceed total limit %d bytes", editsMultipartTotalLimit)
		}
		part, err := writer.CreateFormFile("image", fmt.Sprintf("image-%d%s", i+1, imageFileExtension(imageTypes[i])))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(payload); err != nil {
			return nil, "", err
		}
	}

	if parsed.HasMask && parsed.MaskImageURL != "" {
		maskPayload, maskType, err := fetchEditsImage(ctx, parsed.MaskImageURL)
		if err != nil {
			return nil, "", fmt.Errorf("mask download failed: %w", err)
		}
		part, err := writer.CreateFormFile("mask", "mask"+imageFileExtension(maskType))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(maskPayload); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}
