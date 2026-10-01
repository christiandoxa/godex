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
	best := classifyStatusText(status, body)
	for _, code := range structuredErrorCodes(body) {
		candidate := classifyCode(status, code)
		if errorRank(candidate.Class) < errorRank(best.Class) {
			best = candidate
		}
	}
	return best
}

func RetryableAcrossCredentials(class ErrorClass) bool {
	return class == ErrorAuth || class == ErrorQuota || class == ErrorRateLimit || class == ErrorTransient
}

func RetryableAcrossModels(class ErrorClass) bool {
	return class == ErrorQuota || class == ErrorRateLimit || class == ErrorTransient || class == ErrorNotFound
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
	case status == 500 || status == 502 || status == 503 || status == 504:
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
	switch code {
	case "unauthenticated", "invalid_api_key", "authentication_error":
		return ErrorClassification{Class: ErrorAuth}
	case "insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded", "project_spend_limit_exceeded", "quota_exhausted", "quota_exceeded", "resource_exhausted", "usage_limit_reached":
		return ErrorClassification{Class: ErrorQuota, Cooldown: 5 * time.Minute}
	case "rate_limit_exceeded", "rate_limit_exceeded_error", "slow_down":
		return ErrorClassification{Class: ErrorRateLimit, Cooldown: time.Minute}
	case "model_not_supported":
		return ErrorClassification{Class: ErrorNotFound}
	default:
		return classifyStatusText(status, []byte(code))
	}
}

func structuredErrorCodes(body []byte) []string {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return nil
	}
	codes := make([]string, 0, 4)
	collectErrorCodes(value, &codes)
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
	case "code", "type", "error_code", "status":
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
