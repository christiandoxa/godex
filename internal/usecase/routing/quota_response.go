package routing

import (
	"encoding/json"
	"strings"
)

func isQuotaResponse(body []byte) bool {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	return quotaValue(value)
}

func quotaValue(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if quotaFieldValue(key, child) || quotaValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if quotaValue(child) {
				return true
			}
		}
	}
	return false
}

func quotaFieldValue(key string, value any) bool {
	switch key {
	case "code", "type", "error_code", "status", "reason":
		text, ok := value.(string)
		return ok && quotaCode(text)
	default:
		return false
	}
}

func quotaCode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded",
		"project_spend_limit_exceeded", "quota_exhausted", "quota_exceeded", "resource_exhausted",
		"usage_limit_reached", "usage_not_included", "workspace_member_credits_depleted":
		return true
	default:
		return false
	}
}
