export const imagePricingPlatforms = new Set([
  "antigravity",
  "composite",
  "gemini",
  "grok",
  "openai",
]);

export const supportsImagePricingPlatform = (platform: string): boolean =>
  imagePricingPlatforms.has(platform);

export const supportsVideoPricingPlatform = (platform: string): boolean =>
  platform === "grok";

export const imagePricingI18nKey = (_platform: string, key: string): string =>
  `admin.groups.imagePricing.${key}`;

export const videoPricingI18nKey = (key: string): string =>
  `admin.groups.videoPricing.${key}`;

type ImagePricingTierKey = "image_price_1k" | "image_price_2k" | "image_price_4k";
export type ImageQuality = "low" | "medium" | "high" | "xhigh" | "max";
export type ImageQualityPricingTierKey = "1K" | "2K" | "4K";

export const imageQualityOptions: Array<{ key: ImageQuality; label: string }> = [
  { key: "low", label: "Low" },
  { key: "medium", label: "Medium" },
  { key: "high", label: "High" },
  { key: "xhigh", label: "XHigh" },
  { key: "max", label: "Max" },
];

export const imageQualityPricePlaceholders: Record<
  ImageQuality,
  Record<ImageQualityPricingTierKey, string>
> = {
  low: { "1K": "0.03", "2K": "0.04", "4K": "0.05" },
  medium: { "1K": "0.08", "2K": "0.12", "4K": "0.18" },
  high: { "1K": "0.134", "2K": "0.201", "4K": "0.268" },
  // xhigh/max 暂无官方参考价,留空提示由管理员自行定价。
  xhigh: { "1K": "", "2K": "", "4K": "" },
  max: { "1K": "", "2K": "", "4K": "" },
};
type VideoPricingTierKey =
  | "video_price_480p"
  | "video_price_720p"
  | "video_price_1080p";

const defaultImagePricePlaceholders: Record<
  string,
  Record<ImagePricingTierKey, string>
> = {
  default: {
    image_price_1k: "0.134",
    image_price_2k: "0.201",
    image_price_4k: "0.268",
  },
  grok: {
    image_price_1k: "0.02",
    image_price_2k: "0.02",
    image_price_4k: "0.02",
  },
};

// 视频价为每秒单价（USD/s）。480p/720p 取 grok-imagine-video（文生视频实际走该模型）的
// 官方每秒价；1080p 仅 grok-imagine-video-1.5 图生视频支持，取 1.5 的每秒价。
const defaultVideoPricePlaceholders: Record<
  string,
  Record<VideoPricingTierKey, string>
> = {
  grok: {
    video_price_480p: "0.05",
    video_price_720p: "0.07",
    video_price_1080p: "0.25",
  },
};

export const getImagePricePlaceholder = (
  platform: string,
  tier: ImagePricingTierKey,
): string => {
  const card = defaultImagePricePlaceholders[platform] ?? defaultImagePricePlaceholders.default;
  return card[tier];
};

export const getVideoPricePlaceholder = (
  platform: string,
  tier: VideoPricingTierKey,
): string => {
  const card = defaultVideoPricePlaceholders[platform];
  return card?.[tier] ?? "";
};

export const getDefaultImagePreviewPrice = (
  platform: string,
  tier: ImagePricingTierKey,
): number | null => {
  const placeholder = getImagePricePlaceholder(platform, tier);
  if (placeholder === "") {
    return null;
  }
  const value = Number(placeholder);
  return Number.isFinite(value) ? value : null;
};

export const getDefaultVideoPreviewPrice = (
  platform: string,
  tier: VideoPricingTierKey,
): number | null => {
  const placeholder = getVideoPricePlaceholder(platform, tier);
  if (placeholder === "") {
    return null;
  }
  const value = Number(placeholder);
  return Number.isFinite(value) ? value : null;
};
