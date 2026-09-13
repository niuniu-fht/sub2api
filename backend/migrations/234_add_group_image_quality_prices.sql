-- OpenAI 图片按 quality × size 的按次价格矩阵。
-- 旧 image_price_1k/2k/4k 继续作为未指定 quality 或 auto/未知 quality 的兜底价格。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS image_quality_prices JSONB;

COMMENT ON COLUMN groups.image_quality_prices IS
    'OpenAI 图片按 quality×size 的按次单价 (USD)。key 为 low/medium/high，value 为 1K/2K/4K→价格；NULL/空表示不覆盖';
