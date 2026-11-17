-- OpenAI 生图请求最终返回给客户端的图片 URL（图床转存后的地址）：
-- 仅 /v1/images/generations 与 /v1/images/edits 的非流式响应且存在 http(s)
-- URL 时写入 JSONB 数组，data: 内联 / 纯 b64（未开图床转换）与其余请求为 NULL。
-- 供管理员使用记录核查下游实际拿到的图片地址。
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS image_urls JSONB;
