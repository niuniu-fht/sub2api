import io

p = 'internal/service/openai_images_async_task.go'
s = io.open(p, encoding='utf-8').read()

# 1. 文档示例改为扁平配置格式
old = '''// {
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
// }'''
new = '''// {
//   "base_url": "https://www.faststable.cc",
//   "api_key": "sk-xxx",
//   "headers": {"User-Agent": "Mozilla/5.0"},
//   "create_path": "/v2/images/tasks",
//   "create_body": {
//     "model": "gpt-image-2",
//     "channelId": "gpt-image-default",
//     "params": {"prompt": "{{prompt}}", "count": "{{n}}",
//                "size": "{{size}}", "quality": "{{quality}}"}
//   },
//   "task_id_path": "task_id",
//   "status_path": "/v2/images/tasks/{{task_id}}",
//   "status_path_value": "status",
//   "success_value": "succeeded",
//   "failure_value": "failed",
//   "image_urls_path": "output.images.#.url",
//   "error_message_path": "error.message",
//   "poll_interval_seconds": 5,
//   "timeout_seconds": 300
// }'''
assert old in s, 'doc'
s = s.replace(old, new, 1)

# 2. build request:去掉 client 参数
old = '''func asyncTaskBuildRequest(ctx context.Context, client *http.Client, method, rawURL string, headers map[string]string, apiKey string, body []byte) (*http.Request, error) {'''
new = '''func asyncTaskBuildRequest(ctx context.Context, method, rawURL string, headers map[string]string, apiKey string, body []byte) (*http.Request, error) {'''
assert old in s, 'build-sig'
s = s.replace(old, new, 1)
old = '''	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	_ = client
	return req, nil
}

type asyncTaskHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}
'''
new = '''	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	return req, nil
}
'''
assert old in s, 'doer-iface'
s = s.replace(old, new, 1)

# 3. doer 调用改为 HTTPUpstream.Do
old = '''	doer := asyncTaskHTTPDoer(s.httpUpstream)

	// ---- 1. 创建任务 ----'''
new = '''	// ---- 1. 创建任务 ----'''
assert old in s, 'doer-var'
s = s.replace(old, new, 1)
old = '''	createReq, err := asyncTaskBuildRequest(pollCtx, nil, cfg.CreateMethod, cfg.BaseURL+cfg.CreatePath, cfg.Headers, cfg.APIKey, createBody)
	if err != nil {
		return nil, fmt.Errorf("async task create request: %w", err)
	}
	resp, err := doer.Do(createReq)'''
new = '''	createReq, err := asyncTaskBuildRequest(pollCtx, cfg.CreateMethod, cfg.BaseURL+cfg.CreatePath, cfg.Headers, cfg.APIKey, createBody)
	if err != nil {
		return nil, fmt.Errorf("async task create request: %w", err)
	}
	resp, err := s.httpUpstream.Do(createReq, "", account.ID, account.Concurrency)'''
assert old in s, 'create-do'
s = s.replace(old, new, 1)
old = '''		pollReq, err := asyncTaskBuildRequest(pollCtx, nil, cfg.StatusMethod, cfg.BaseURL+statusPath, cfg.Headers, cfg.APIKey, nil)'''
new = '''		pollReq, err := asyncTaskBuildRequest(pollCtx, cfg.StatusMethod, cfg.BaseURL+statusPath, cfg.Headers, cfg.APIKey, nil)'''
assert old in s, 'poll-req'
s = s.replace(old, new, 1)
old = '''		pollResp, err := doer.Do(pollReq)'''
new = '''		pollResp, err := s.httpUpstream.Do(pollReq, "", account.ID, account.Concurrency)'''
assert old in s, 'poll-do'
s = s.replace(old, new, 1)

# 4. StatusPathTpl -> StatusPath
old = '''	statusPath := strings.ReplaceAll(cfg.StatusPathTpl, "{{task_id}}", taskID)'''
new = '''	statusPath := strings.ReplaceAll(cfg.StatusPath, "{{task_id}}", taskID)'''
assert old in s, 'statuspath'
s = s.replace(old, new, 1)

# 5. 移除 replaced map(已无读取点)
old = '''	seen := make(map[string]struct{})
	replaced := make(map[string]struct{})
'''
new = '''	seen := make(map[string]struct{})
'''
assert old in s, 'replaced-decl'
s = s.replace(old, new, 1)
old = '''				replaced[formName] = struct{}{}
				applyOpenAIImageParsedDefault(parsed, formName, defaultValue, true)'''
new = '''				applyOpenAIImageParsedDefault(parsed, formName, defaultValue, true)'''
assert old in s, 'replaced-write'
s = s.replace(old, new, 1)

# 6. 修正 finish 签名中未用的 createResp 命名一致性 & imports
old = '''import (
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
)'''
new = '''import (
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
	responseheaders "github.com/Wei-Shaw/sub2api/internal/pkg/responseheaders"
)'''
assert old in s, 'imports'
s = s.replace(old, new, 1)

io.open(p, 'w', encoding='utf-8', newline='').write(s)
print('all fixes applied')
