-- 请求内账号尝试链：仅发生过账号切换（链长>1）的行写入 JSONB，其余行为 NULL。
-- 供管理员使用记录展示「账号尝试次数」与尝试链路详情。
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS account_attempts JSONB;
