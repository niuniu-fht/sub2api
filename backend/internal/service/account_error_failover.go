package service

import (
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	credKeyErrorFailoverEnabled = "error_failover_enabled"
	credKeyErrorFailoverRules   = "error_failover_rules"

	maxErrorFailoverRules       = 20
	maxErrorFailoverStatusCodes = 20
	maxErrorFailoverKeywords    = 50
	maxErrorFailoverKeywordLen  = 200
)

// NormalizeErrorFailoverCredentials 校验 credentials 中的图片错误自动切换策略字段。
// 供账号创建/更新/批量更新的保存路径调用；credentials 未携带相关字段时为 no-op。
// 校验内容：enabled 必须为布尔；rules 必须为对象数组，每条规则至少配置一个
// 状态码或关键词，状态码取值 100-599，数量上限防止误配超大规则集。
func NormalizeErrorFailoverCredentials(credentials map[string]any) error {
	if credentials == nil {
		return nil
	}
	if raw, ok := credentials[credKeyErrorFailoverEnabled]; ok && raw != nil {
		if _, isBool := raw.(bool); !isBool {
			return infraerrors.New(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
				"error_failover_enabled must be a boolean")
		}
	}
	raw, ok := credentials[credKeyErrorFailoverRules]
	if !ok || raw == nil {
		return nil
	}

	arr, isSlice := raw.([]any)
	if !isSlice {
		return infraerrors.New(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
			"error_failover_rules must be an array of rule objects")
	}
	if len(arr) > maxErrorFailoverRules {
		return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
			"error_failover_rules supports at most %d rules", maxErrorFailoverRules)
	}

	for i, item := range arr {
		entry, isMap := item.(map[string]any)
		if !isMap {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
				"error_failover_rules[%d] must be an object", i)
		}

		hasStatusCodes := false
		if rawCodes, exists := entry["status_codes"]; exists && rawCodes != nil {
			codes, isArr := rawCodes.([]any)
			if !isArr {
				return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
					"error_failover_rules[%d].status_codes must be an array of integers", i)
			}
			if len(codes) > maxErrorFailoverStatusCodes {
				return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
					"error_failover_rules[%d].status_codes supports at most %d entries", i, maxErrorFailoverStatusCodes)
			}
			for _, rawCode := range codes {
				code := parseTempUnschedInt(rawCode)
				if code < 100 || code > 599 {
					return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
						"error_failover_rules[%d].status_codes contains invalid HTTP status code %v", i, rawCode)
				}
			}
			hasStatusCodes = len(codes) > 0
		}

		hasKeywords := false
		if rawKeywords, exists := entry["keywords"]; exists && rawKeywords != nil {
			keywords, isArr := rawKeywords.([]any)
			if !isArr {
				return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
					"error_failover_rules[%d].keywords must be an array of strings", i)
			}
			if len(keywords) > maxErrorFailoverKeywords {
				return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
					"error_failover_rules[%d].keywords supports at most %d entries", i, maxErrorFailoverKeywords)
			}
			for _, rawKeyword := range keywords {
				keyword, isStr := rawKeyword.(string)
				if !isStr {
					return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
						"error_failover_rules[%d].keywords must be an array of strings", i)
				}
				if len(keyword) > maxErrorFailoverKeywordLen {
					return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
						"error_failover_rules[%d].keywords entry too long (max %d chars)", i, maxErrorFailoverKeywordLen)
				}
			}
			hasKeywords = len(keywords) > 0
		}

		if !hasStatusCodes && !hasKeywords {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_ERROR_FAILOVER",
				"error_failover_rules[%d] must configure status_codes and/or keywords", i)
		}
	}
	return nil
}
