package service

import (
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	DefaultImageBilling2KPixelThreshold int64 = 3000000
	DefaultImageBilling4KPixelThreshold int64 = 6000000

	ImageBillingSize1K = "1K"
	ImageBillingSize2K = "2K"
	ImageBillingSize4K = "4K"

	ImageSizeSourceOutput  = "output"
	ImageSizeSourceInput   = "input"
	ImageSizeSourceDefault = "default"
	ImageSizeSourceLegacy  = "legacy"
)

type ImageBillingSizeResolution struct {
	BillingSize string
	InputSize   string
	OutputSize  string
	Source      string
	Breakdown   map[string]int
}

// ImageBillingAreaThresholds controls area-based image billing tier classification.
// A request is 2K when width*height > TwoKPixelThreshold, and 4K when
// width*height > FourKPixelThreshold. Otherwise it remains 1K.
type ImageBillingAreaThresholds struct {
	TwoKPixelThreshold  int64 `json:"two_k_pixel_threshold"`
	FourKPixelThreshold int64 `json:"four_k_pixel_threshold"`
}

var imageBillingAreaThresholdsStore atomic.Value // stores ImageBillingAreaThresholds

func DefaultImageBillingAreaThresholds() ImageBillingAreaThresholds {
	return ImageBillingAreaThresholds{
		TwoKPixelThreshold:  DefaultImageBilling2KPixelThreshold,
		FourKPixelThreshold: DefaultImageBilling4KPixelThreshold,
	}
}

func NormalizeImageBillingAreaThresholds(twoKPixelThreshold int64, fourKPixelThreshold int64) ImageBillingAreaThresholds {
	if twoKPixelThreshold <= 0 {
		twoKPixelThreshold = DefaultImageBilling2KPixelThreshold
	}
	if fourKPixelThreshold <= 0 {
		fourKPixelThreshold = DefaultImageBilling4KPixelThreshold
	}
	if fourKPixelThreshold <= twoKPixelThreshold {
		fourKPixelThreshold = twoKPixelThreshold * 2
	}
	return ImageBillingAreaThresholds{
		TwoKPixelThreshold:  twoKPixelThreshold,
		FourKPixelThreshold: fourKPixelThreshold,
	}
}

func SetImageBillingAreaThresholds(twoKPixelThreshold int64, fourKPixelThreshold int64) ImageBillingAreaThresholds {
	thresholds := NormalizeImageBillingAreaThresholds(twoKPixelThreshold, fourKPixelThreshold)
	imageBillingAreaThresholdsStore.Store(thresholds)
	return thresholds
}

func CurrentImageBillingAreaThresholds() ImageBillingAreaThresholds {
	if value := imageBillingAreaThresholdsStore.Load(); value != nil {
		if thresholds, ok := value.(ImageBillingAreaThresholds); ok {
			return NormalizeImageBillingAreaThresholds(thresholds.TwoKPixelThreshold, thresholds.FourKPixelThreshold)
		}
	}
	return DefaultImageBillingAreaThresholds()
}

func ClassifyImageBillingTier(size string) (string, bool) {
	trimmed := strings.TrimSpace(size)
	normalized := strings.ToLower(trimmed)
	switch normalized {
	case "", "auto":
		return "", false
	case "1k":
		return ImageBillingSize1K, true
	case "2k":
		return ImageBillingSize2K, true
	case "4k":
		return ImageBillingSize4K, true
	}

	width, height, ok := parseImageBillingDimensions(trimmed)
	if !ok {
		return "", false
	}

	thresholds := CurrentImageBillingAreaThresholds()
	switch {
	case imageBillingAreaGreaterThan(width, height, thresholds.FourKPixelThreshold):
		return ImageBillingSize4K, true
	case imageBillingAreaGreaterThan(width, height, thresholds.TwoKPixelThreshold):
		return ImageBillingSize2K, true
	default:
		return ImageBillingSize1K, true
	}
}

func imageBillingAreaGreaterThan(width int, height int, threshold int64) bool {
	if width <= 0 || height <= 0 || threshold < 0 {
		return false
	}
	return int64(width) > threshold/int64(height)
}

func NormalizeImageBillingTierOrDefault(size string) string {
	if tier, ok := ClassifyImageBillingTier(size); ok {
		return tier
	}
	return ImageBillingSize1K
}

func ResolveImageBillingSize(inputSize string, outputSizes []string) ImageBillingSizeResolution {
	inputSize = strings.TrimSpace(inputSize)
	outputSizes = compactTrimmedStrings(outputSizes)

	breakdown := map[string]int{}
	outputSize := firstDisplayImageOutputSize(outputSizes)
	outputTier := ""
	for _, output := range outputSizes {
		tier, ok := ClassifyImageBillingTier(output)
		if !ok {
			continue
		}
		breakdown[tier]++
		if imageTierRank(tier) > imageTierRank(outputTier) {
			outputTier = tier
		}
	}
	if outputTier != "" {
		return ImageBillingSizeResolution{
			BillingSize: outputTier,
			InputSize:   inputSize,
			OutputSize:  outputSize,
			Source:      ImageSizeSourceOutput,
			Breakdown:   normalizeImageSizeBreakdown(breakdown),
		}
	}

	if tier, ok := ClassifyImageBillingTier(inputSize); ok {
		return ImageBillingSizeResolution{
			BillingSize: tier,
			InputSize:   inputSize,
			OutputSize:  outputSize,
			Source:      ImageSizeSourceInput,
		}
	}

	return ImageBillingSizeResolution{
		BillingSize: ImageBillingSize1K,
		InputSize:   inputSize,
		OutputSize:  outputSize,
		Source:      ImageSizeSourceDefault,
	}
}

func ApplyOpenAIImageBillingResolution(result *OpenAIForwardResult) {
	if result == nil || result.ImageCount <= 0 {
		return
	}
	inputSize := strings.TrimSpace(result.ImageInputSize)
	if inputSize == "" {
		inputSize = strings.TrimSpace(result.ImageSize)
	}
	outputSizes := result.ImageOutputSizes
	if len(outputSizes) == 0 && strings.TrimSpace(result.ImageOutputSize) != "" {
		outputSizes = []string{result.ImageOutputSize}
	}
	resolved := ResolveImageBillingSize(inputSize, outputSizes)
	applyImageBillingResolution(
		&result.ImageSize,
		&result.ImageInputSize,
		&result.ImageOutputSize,
		&result.ImageSizeSource,
		&result.ImageSizeBreakdown,
		resolved,
	)
}

func ApplyForwardImageBillingResolution(result *ForwardResult) {
	if result == nil || result.ImageCount <= 0 {
		return
	}
	inputSize := strings.TrimSpace(result.ImageInputSize)
	if inputSize == "" {
		inputSize = strings.TrimSpace(result.ImageSize)
	}
	outputSizes := result.ImageOutputSizes
	if len(outputSizes) == 0 && strings.TrimSpace(result.ImageOutputSize) != "" {
		outputSizes = []string{result.ImageOutputSize}
	}
	resolved := ResolveImageBillingSize(inputSize, outputSizes)
	applyImageBillingResolution(
		&result.ImageSize,
		&result.ImageInputSize,
		&result.ImageOutputSize,
		&result.ImageSizeSource,
		&result.ImageSizeBreakdown,
		resolved,
	)
}

func applyImageBillingResolution(
	billingSize *string,
	inputSize *string,
	outputSize *string,
	source *string,
	breakdown *map[string]int,
	resolved ImageBillingSizeResolution,
) {
	*billingSize = resolved.BillingSize
	*inputSize = resolved.InputSize
	*outputSize = resolved.OutputSize
	*source = resolved.Source
	*breakdown = resolved.Breakdown
}

func parseImageBillingDimensions(size string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width64, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	height64, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	maxInt := int64(^uint(0) >> 1)
	if width64 <= 0 || height64 <= 0 || width64 > maxInt || height64 > maxInt {
		return 0, 0, false
	}
	return int(width64), int(height64), true
}

func compactTrimmedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func firstDisplayImageOutputSize(outputSizes []string) string {
	for _, output := range outputSizes {
		if trimmed := strings.TrimSpace(output); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func imageTierRank(tier string) int {
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case ImageBillingSize1K:
		return 1
	case ImageBillingSize2K:
		return 2
	case ImageBillingSize4K:
		return 3
	default:
		return 0
	}
}

func normalizeImageSizeBreakdown(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
		if count := in[tier]; count > 0 {
			out[tier] = count
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func SortedImageBillingBreakdownKeys(breakdown map[string]int) []string {
	keys := make([]string, 0, len(breakdown))
	for key := range breakdown {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := imageTierRank(keys[i]), imageTierRank(keys[j])
		if left == right {
			return keys[i] < keys[j]
		}
		return left < right
	})
	return keys
}
