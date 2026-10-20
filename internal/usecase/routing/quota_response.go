package routing

import "encoding/json"

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
	case "code", "type", "error", "error_code", "status", "reason":
		text, ok := value.(string)
		return ok && quotaCode(text)
	default:
		return false
	}
}

func quotaCode(value string) bool {
	// 0.438.3's runtime_error_code_class uses exact untrimmed ASCII-only
	// byte comparison. Unicode lowercasing (e.g. worKspace) would otherwise
	// invent quota evidence in an untrusted HTTP 402/403 or SSE payload.
	switch {
	case previousASCIIEqualFold04383(value, "insufficient_quota"),
		previousASCIIEqualFold04383(value, "credit_balance_exhausted"),
		previousASCIIEqualFold04383(value, "organization_spend_limit_exceeded"),
		previousASCIIEqualFold04383(value, "project_spend_limit_exceeded"),
		previousASCIIEqualFold04383(value, "quota_exhausted"),
		previousASCIIEqualFold04383(value, "quota_exceeded"),
		previousASCIIEqualFold04383(value, "resource_exhausted"),
		previousASCIIEqualFold04383(value, "usage_limit_reached"),
		previousASCIIEqualFold04383(value, "usage_not_included"),
		previousASCIIEqualFold04383(value, "workspace_member_credits_depleted"):
		return true
	default:
		return false
	}
}
