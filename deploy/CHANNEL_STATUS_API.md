# 渠道状态 API（GET /api/v1/channel-status）

> 面向外部系统的只读接口：拉取各渠道（上游账号）的可用性快照。
> 数据来自后台对真实生成记录（用量日志 + 错误日志）的定时聚合（默认每 60s 刷新一次，可在管理后台配置），
> 非实时查询，匿名可访问（管理后台开启「渠道状态统计」后可用，关闭时返回 404）。

## 内置状态页

`/status`（如 `https://你的域名/status`）是消费此接口的内置匿名页面，无需登录即可访问：
整体状态横幅 + 各渠道成功率/平均生图时间/24 格时间线 + 可展开的按模型明细，自动刷新。
功能关闭时页面显示「未启用」提示。外部系统亦可按下方契约自行对接。

## 请求

```
GET /api/v1/channel-status
```

无认证、无参数。限流：按客户端 IP（超出返回 429）。

## 响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "generated_at_ms": 1794316743023,
    "window_hours": 24,
    "refresh_seconds": 60,
    "overall": "down",
    "channels": [
      {
        "id": 256,
        "name": "某中转站",
        "platform": "openai",
        "status": "degraded",
        "requests": 5,
        "failures": 56,
        "success_rate": 8.2,
        "avg_gen_ms": 64220,
        "models": [
          {
            "model": "gpt-image-2.5-sunburst",
            "requests": 1,
            "failures": 41,
            "success_rate": 2.38,
            "avg_gen_ms": 61000
          }
        ],
        "timeline": [
          {
            "bucket_start_ms": 1794230400000,
            "requests": 2,
            "failures": 5,
            "success_rate": 28.57,
            "status": "down"
          }
        ]
      }
    ]
  }
}
```

### 字段说明

| 字段 | 说明 |
|---|---|
| `generated_at_ms` | 快照生成时间（毫秒时间戳）；消费方可据此判断数据新鲜度 |
| `window_hours` | 统计窗口（小时）：成功率/平均耗时覆盖最近 N 小时 |
| `refresh_seconds` | 聚合刷新间隔（秒）：数据最大滞后时间 |
| `overall` | 全局状态：`operational` / `degraded` / `down` / `unknown`（取所有渠道最差值；无数据为 unknown） |
| `channels[].status` | 渠道状态：`operational`（成功率≥健康阈值）/ `degraded`（≥降级阈值）/ `down` / `unknown`（窗口内无请求） |
| `success_rate` | 成功率百分比（0-100）；`-1` 表示窗口内无请求（unknown） |
| `failures` | 过滤后仍计入的失败数；命中后台「失败过滤规则」（状态码/错误关键词）的失败已被排除 |
| `avg_gen_ms` | 成功请求的平均耗时（毫秒），即平均生图时间；0 表示窗口内无成功样本 |
| `models[]` | 按模型细分的同口径统计，按请求量降序 |
| `timeline[]` | 24 个时间桶（桶宽=窗口/24），供 uptime 条展示；`bucket_start_ms` 为桶起点 |
| 首轮聚合未完成时 | 返回 `ready:false` + 空 `channels`，消费方应轮询等待 |

## 管理端配置

- `GET /api/v1/admin/settings/channel-status` / `PUT`（管理员 JWT）：开关 `enabled` + 配置
  `config`（`window_hours` / `refresh_seconds` / `filter_rules[{status_codes,keywords}]` / `thresholds`）
- 过滤语义：失败记录的上游状态码命中 `status_codes`（或上游状态码为 0 时回退客户端状态码）且/或错误消息包含任一
  `keywords`（不区分大小写）→ 该次失败不计入。规则内维度 AND，规则间 OR。
- 保存配置后服务端立即触发一轮聚合（无需等刷新间隔）。
