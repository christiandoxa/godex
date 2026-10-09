package provider

import (
	"encoding/json"
	"strings"
	"time"
)

type ErrorClass uint8

const (
	ErrorAuth ErrorClass = iota
	ErrorQuota
	ErrorRateLimit
	ErrorTransient
	ErrorNotFound
	ErrorOther
)

type ErrorClassification struct {
	Class    ErrorClass
	Cooldown time.Duration
}

func ClassifyError(status int, body []byte) ErrorClassification {
	if status == 429 {
		if class, ok := gemini429Class(body); ok {
			if class == ErrorQuota {
				return ErrorClassification{Class: class, Cooldown: 5 * time.Minute}
			}
			return ErrorClassification{Class: class, Cooldown: time.Minute}
		}
	}
	best := classifyStatusText(status, body)
	for _, code := range structuredErrorCodes(body) {
		candidate := classifyCode(status, code)
		if errorRank(candidate.Class) < errorRank(best.Class) {
			best = candidate
		}
	}
	return best
}

func ClassifyProviderCode(code string) ErrorClassification {
	classification, ok := classifyProviderCode(code)
	if !ok {
		return ErrorClassification{Class: ErrorOther}
	}
	return classification
}

func ClassifyFirstEventError(body []byte) (string, ErrorClassification, bool) {
	var event map[string]any
	if json.Unmarshal(body, &event) != nil || event == nil {
		return "", ErrorClassification{Class: ErrorOther}, false
	}
	errorValue, hasError := event["error"]
	if event["type"] != "error" && !hasError {
		return "", ErrorClassification{Class: ErrorOther}, false
	}
	var code string
	var found bool
	if providerError, ok := errorValue.(map[string]any); ok {
		code, found = providerError["type"].(string)
		if !found {
			code, found = providerError["code"].(string)
		}
	}
	if !found {
		code, found = event["code"].(string)
	}
	if !found {
		return "", ErrorClassification{Class: ErrorOther}, true
	}
	return code, ClassifyProviderCode(code), true
}

func RetryableAcrossCredentials(class ErrorClass) bool {
	return class == ErrorAuth || class == ErrorQuota || class == ErrorRateLimit || class == ErrorTransient
}

func RetryableAcrossModels(class ErrorClass) bool {
	return class == ErrorQuota || class == ErrorRateLimit || class == ErrorTransient || class == ErrorNotFound
}

func IsStructuredGemini429(body []byte) bool {
	_, ok := gemini429Class(body)
	return ok
}

func gemini429Class(body []byte) (ErrorClass, bool) {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return ErrorOther, false
	}
	return gemini429Value(value)
}

func gemini429Value(value any) (ErrorClass, bool) {
	class := ErrorOther
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"status", "code", "reason"} {
			if code, ok := typed[key].(string); ok {
				switch {
				case gemini429QuotaCode(code):
					class = ErrorQuota
				case gemini429RateCode(code) && class != ErrorQuota:
					class = ErrorRateLimit
				}
			}
		}
		for _, key := range []string{"quotaId", "quota_limit", "quotaLimit"} {
			if quota, ok := typed[key].(string); ok && (strings.Contains(quota, "PerDay") || strings.Contains(quota, "Daily")) {
				class = ErrorQuota
			}
		}
		for _, child := range typed {
			if childClass, found := gemini429Value(child); found {
				if childClass == ErrorQuota || class == ErrorOther {
					class = childClass
				}
			}
		}
	case []any:
		for _, child := range typed {
			if childClass, found := gemini429Value(child); found {
				if childClass == ErrorQuota || class == ErrorOther {
					class = childClass
				}
			}
		}
	}
	return class, class != ErrorOther
}

func gemini429QuotaCode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "quota_exhausted", "quota_exceeded", "resource_exhausted", "insufficient_g1_credits_balance", "insufficient_quota":
		return true
	default:
		return false
	}
}

func gemini429RateCode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "rate_limit_exceeded", "rate_limit_exceeded_error":
		return true
	default:
		return false
	}
}

func classifyStatusText(status int, body []byte) ErrorClassification {
	if status == 429 {
		return ErrorClassification{Class: ErrorOther}
	}
	text := strings.ToLower(string(body))
	switch {
	case status == 401 || status == 403:
		return ErrorClassification{Class: ErrorAuth}
	case status == 404:
		return ErrorClassification{Class: ErrorNotFound}
	case status == 500 || status == 502 || status == 503 || status == 504 || status == 529:
		return ErrorClassification{Class: ErrorTransient, Cooldown: 10 * time.Second}
	case strings.Contains(text, "model is not supported"):
		return ErrorClassification{Class: ErrorNotFound}
	case strings.Contains(text, "overloaded"):
		return ErrorClassification{Class: ErrorTransient, Cooldown: 10 * time.Second}
	default:
		return ErrorClassification{Class: ErrorOther}
	}
}

func classifyCode(status int, code string) ErrorClassification {
	code = strings.ToLower(strings.TrimSpace(code))
	if status != 429 && (status == 401 || status == 403) {
		return ErrorClassification{Class: ErrorAuth}
	}
	if classification, ok := classifyProviderCode(code); ok {
		return classification
	}
	return classifyStatusText(status, []byte(code))
}

func classifyProviderCode(code string) (ErrorClassification, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "unauthenticated", "unauthorized", "invalid_api_key", "authentication_error":
		return ErrorClassification{Class: ErrorAuth}, true
	case "insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded", "project_spend_limit_exceeded", "quota_exhausted", "quota_exceeded", "resource_exhausted", "usage_limit_reached", "usage_not_included", "workspace_member_credits_depleted":
		return ErrorClassification{Class: ErrorQuota, Cooldown: 5 * time.Minute}, true
	case "rate_limit_error", "rate_limit_exceeded", "rate_limit_exceeded_error", "slow_down":
		return ErrorClassification{Class: ErrorRateLimit, Cooldown: time.Minute}, true
	case "not_found_error", "model_not_supported":
		return ErrorClassification{Class: ErrorNotFound}, true
	case "overloaded_error", "server_is_overloaded":
		return ErrorClassification{Class: ErrorTransient, Cooldown: 10 * time.Second}, true
	default:
		return ErrorClassification{Class: ErrorOther}, false
	}
}

func structuredErrorCodes(body []byte) []string {
	var value any
	if json.Unmarshal(body, &value) == nil {
		codes := make([]string, 0, 4)
		collectErrorCodes(value, &codes)
		return codes
	}
	codes := make([]string, 0, 4)
	for _, line := range strings.Split(string(body), "\n") {
		payload, found := strings.CutPrefix(strings.TrimSpace(line), "data:")
		if !found || json.Unmarshal([]byte(strings.TrimSpace(payload)), &value) != nil {
			continue
		}
		collectErrorCodes(value, &codes)
	}
	return codes
}

func collectErrorCodes(value any, codes *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isErrorCodeField(key) {
				if text, ok := child.(string); ok && text != "" {
					*codes = append(*codes, text)
				}
			}
			collectErrorCodes(child, codes)
		}
	case []any:
		for _, child := range typed {
			collectErrorCodes(child, codes)
		}
	}
}

func isErrorCodeField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "code", "type", "error_code", "reason", "status":
		return true
	default:
		return false
	}
}

func errorRank(class ErrorClass) int {
	switch class {
	case ErrorAuth:
		return 0
	case ErrorQuota:
		return 1
	case ErrorRateLimit:
		return 2
	case ErrorNotFound:
		return 3
	case ErrorTransient:
		return 4
	default:
		return 5
	}
}
