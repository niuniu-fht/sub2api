package service

func imagePriceConfigFromAPIKey(apiKey *APIKey) *ImagePriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return &ImagePriceConfig{
		Price1K:       apiKey.Group.ImagePrice1K,
		Price2K:       apiKey.Group.ImagePrice2K,
		Price4K:       apiKey.Group.ImagePrice4K,
		QualityPrices: NormalizeImageQualityPrices(apiKey.Group.ImageQualityPrices),
	}
}

func apiKeyHasConfiguredImagePrice(apiKey *APIKey, imageSize string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetImagePrice(imageSize) != nil
}

func apiKeyHasConfiguredImageQualityPrice(apiKey *APIKey, quality string, imageSize string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetImageQualityPrice(quality, imageSize) != nil
}

func LookupImageQualityPrice(config *ImagePriceConfig, quality string, imageSize string) *float64 {
	if config == nil || config.QualityPrices == nil {
		return nil
	}
	quality = NormalizeOpenAIImageQuality(quality)
	if quality == "" {
		return nil
	}
	tier := NormalizeImageBillingTierOrDefault(imageSize)
	prices, ok := config.QualityPrices[quality]
	if !ok {
		return nil
	}
	price, ok := prices[tier]
	if !ok || price < 0 {
		return nil
	}
	return &price
}

func NormalizeImageQualityPrices(in map[string]map[string]float64) map[string]map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]map[string]float64, len(in))
	for rawQuality, tiers := range in {
		quality := NormalizeOpenAIImageQuality(rawQuality)
		if quality == "" {
			continue
		}
		if out[quality] == nil {
			out[quality] = make(map[string]float64, len(tiers))
		}
		for rawTier, price := range tiers {
			tier := NormalizeImageBillingTierOrDefault(rawTier)
			if tier == "" || price < 0 {
				continue
			}
			out[quality][tier] = price
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func videoPriceConfigFromAPIKey(apiKey *APIKey) *VideoPriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return &VideoPriceConfig{
		Price480P:   apiKey.Group.VideoPrice480P,
		Price720P:   apiKey.Group.VideoPrice720P,
		Price1080P:  apiKey.Group.VideoPrice1080P,
		ModelPrices: apiKey.Group.VideoModelPrices,
	}
}

func apiKeyHasConfiguredVideoPrice(apiKey *APIKey, model, resolution string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetVideoPriceForModel(model, resolution) != nil
}

func webSearchPricePerCallFromAPIKey(apiKey *APIKey) *float64 {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return apiKey.Group.WebSearchPricePerCall
}

func groupSearchPricePer1kFromAPIKey(apiKey *APIKey) *float64 {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return apiKey.Group.GetSearchPricePer1k()
}

func groupAudioPriceConfigFromAPIKey(apiKey *APIKey) *audioPriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	g := apiKey.Group
	return &audioPriceConfig{
		RealtimePerMin: g.AudioRealtimePricePerMin,
		TTSPerMChars:   g.AudioTTSPricePerMillionChars,
		STTPerHour:     g.AudioSTTPricePerHour,
	}
}
