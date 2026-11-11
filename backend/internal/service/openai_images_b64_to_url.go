package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 账户级开关(Extra):
//   - images_b64_to_url:上游返回 b64_json(或 url)时,网关把图片转存到对象存储,
//     并用存储 URL 覆盖 data[i].url —— 即使上游已带 url 也覆盖。
//   - images_hide_b64:从响应中移除 b64_json 字段,减小响应体。
//     依赖 images_b64_to_url(没有 URL 就删 b64 会让客户端拿不到图)。
const (
	AccountExtraImagesB64ToURL = "images_b64_to_url"
	AccountExtraImagesHideB64  = "images_hide_b64"
)

var imageUploaderResolverForSync func() (*ImageResultUploader, bool)

// RegisterSyncImageUploaderResolver 由启动装配层调用,把后台设置驱动的 uploader
// 解析器发布给同步 Images 路径复用(与异步 ImageTaskService 同一来源)。
func RegisterSyncImageUploaderResolver(r func() (*ImageResultUploader, bool)) {
	imageUploaderResolverForSync = r
}

func syncImageUploader() *ImageResultUploader {
	if imageUploaderResolverForSync == nil {
		return nil
	}
	uploader, _ := imageUploaderResolverForSync()
	return uploader
}

func imagesB64ToURLEnabled(account *Account) bool {
	return account != nil && account.getExtraBool(AccountExtraImagesB64ToURL)
}

func imagesHideB64Enabled(account *Account) bool {
	return account != nil && account.getExtraBool(AccountExtraImagesHideB64)
}

// b64ToURLRetryAttempts 下载/转存的可重试次数（含首次）。网络抖动重试一次；
// 超限/数据损坏这类确定性错误重试也会失败，代价仅为重复读一次。
const b64ToURLRetryAttempts = 2

// rewriteOpenAIImagesB64ToURL 对 Images 端点的非流式响应做「b64→图床 URL」改写:
// 每张图(b64_json 或 url 任一存在)转存到对象存储,url 字段以图床地址覆盖;
// images_hide_b64 开启时同时移除 b64_json。
//
// 失败语义：功能开关关闭或对象存储未启用时原样透传（此时本就不承诺转存）；
// 开关开启后转存是硬性承诺——下载/上传经重试仍失败的项会让整个响应以错误返回,
// 绝不把上游 URL 原样透给客户端（上游 URL 有时效，泄露出去图会失效）。
func (s *OpenAIGatewayService) rewriteOpenAIImagesB64ToURL(
	ctx context.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	body []byte,
) ([]byte, error) {
	if !imagesB64ToURLEnabled(account) {
		return body, nil
	}
	uploader := syncImageUploader()
	if uploader == nil || uploader.storage == nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images b64->url skipped account_id=%d: image storage disabled", account.ID)
		return body, nil
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, nil
	}
	items := gjson.GetBytes(body, "data")
	if !items.IsArray() {
		return body, nil
	}
	hideB64 := imagesHideB64Enabled(account)
	syncRewriteRequestID := strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	for index, item := range items.Array() {
		if !item.IsObject() {
			continue
		}
		b64 := strings.TrimSpace(item.Get("b64_json").String())
		rawURL := strings.TrimSpace(item.Get("url").String())
		if b64 == "" && rawURL == "" {
			continue
		}

		var payload []byte
		var contentType string
		var err error

		if b64 != "" {
			payload, err = base64.StdEncoding.DecodeString(b64)
			if err != nil {
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images b64->url failed account_id=%d index=%d err=decode b64: %s", account.ID, index, err.Error())
				return nil, fmt.Errorf("image re-host failed: decode b64 (index %d)", index)
			}
			contentType = sniffImageContentType(payload)
		} else {
			// 上游只给了 url:复用回填功能的下载管线(校验/代理/私网拦截),再解码为字节。
			// 大小上限跟随图片存储设置的单图下载上限(max_download_bytes),而非回填的固定 20MiB,
			// 避免大图(4K/High)在这里被拒。
			var encoded string
			var fetchErr error
			for attempt := 0; attempt < b64ToURLRetryAttempts; attempt++ {
				encoded, fetchErr = s.fetchOpenAIImageURLBase64WithLimit(ctx, account, rawURL, uploader.maxDownloadBytes)
				if fetchErr == nil {
					break
				}
			}
			if fetchErr != nil {
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images b64->url failed account_id=%d index=%d attempts=%d err=%s", account.ID, index, b64ToURLRetryAttempts, fetchErr.Error())
				return nil, fmt.Errorf("image re-host failed: download upstream image (index %d)", index)
			}
			payload, err = base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images b64->url failed account_id=%d index=%d err=decode fetched: %s", account.ID, index, err.Error())
				return nil, fmt.Errorf("image re-host failed: decode downloaded image (index %d)", index)
			}
			contentType = sniffImageContentType(payload)
		}

		// 每次改写调用生成随机 ID:键必须请求间唯一,否则并发请求互相覆盖(串图),
		// 且确定性键可被枚举遍历他人图片。
		key := uploader.buildKey(fmt.Sprintf("sync-%s-%d", syncRewriteRequestID, index), index, contentType)
		var storedURL string
		var saveErr error
		for attempt := 0; attempt < b64ToURLRetryAttempts; attempt++ {
			storedURL, saveErr = uploader.storage.Save(ctx, key, contentType, payload)
			if saveErr == nil {
				break
			}
		}
		if saveErr != nil {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images b64->url failed account_id=%d index=%d attempts=%d err=upload: %s", account.ID, index, b64ToURLRetryAttempts, saveErr.Error())
			return nil, fmt.Errorf("image re-host failed: store image (index %d)", index)
		}

		var setErr error
		body, setErr = sjson.SetBytes(body, fmt.Sprintf("data.%d.url", index), storedURL)
		if setErr != nil {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images b64->url failed account_id=%d index=%d err=set url: %s", account.ID, index, setErr.Error())
			return nil, fmt.Errorf("image re-host failed: rewrite response (index %d)", index)
		}
		if hideB64 && b64 != "" {
			body, setErr = sjson.DeleteBytes(body, fmt.Sprintf("data.%d.b64_json", index))
			if setErr != nil {
				// 转存已完成,仅隐藏 b64 失败不影响"必须走图床"的承诺,记日志继续。
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images hide b64 account_id=%d index=%d err=%s", account.ID, index, setErr.Error())
			}
		}
	}
	return body, nil
}

func sniffImageContentType(payload []byte) string {
	switch {
	case len(payload) > 3 && payload[0] == 0x89 && payload[1] == 'P' && payload[2] == 'N' && payload[3] == 'G':
		return "image/png"
	case len(payload) > 2 && payload[0] == 0xFF && payload[1] == 0xD8:
		return "image/jpeg"
	case len(payload) > 11 && string(payload[:4]) == "RIFF" && string(payload[8:12]) == "WEBP":
		return "image/webp"
	case len(payload) > 3 && string(payload[:3]) == "GIF":
		return "image/gif"
	default:
		return "image/png"
	}
}
