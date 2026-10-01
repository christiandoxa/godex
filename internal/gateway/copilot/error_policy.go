package copilot

import (
	"bytes"
	"encoding/json"
	"strings"
)

type copilotProviderErrorClass uint8

const (
	copilotErrorAuth copilotProviderErrorClass = iota
	copilotErrorQuota
	copilotErrorRateLimit
	copilotErrorNotFound
	copilotErrorTransient
	copilotErrorOther
)

func copilotModelRetryAllowed(status int, body []byte) bool {
	class := classifyCopilotProviderError(status, body)
	if status == 429 {
		return class == copilotErrorQuota || class == copilotErrorRateLimit || class == copilotErrorTransient
	}
	return class == copilotErrorQuota || class == copilotErrorRateLimit || class == copilotErrorTransient || class == copilotErrorNotFound
}

func classifyCopilotProviderError(status int, body []byte) copilotProviderErrorClass {
	class := classifyCopilotErrorSignal(status, "", string(body), status != 429)
	for _, token := range copilotErrorCodes(body) {
		candidate := classifyCopilotErrorSignal(0, token, "", false)
		if candidate < class {
			class = candidate
		}
	}
	if class == copilotErrorOther && status >= 500 {
		return copilotErrorTransient
	}
	return class
}

func classifyCopilotErrorSignal(status int, code, text string, useStatus bool) copilotProviderErrorClass {
	code = strings.ToLower(strings.TrimSpace(code))
	text = strings.ToLower(text)
	if (useStatus && (status == 401 || status == 403)) || oneOf(code, "unauthenticated", "invalid_api_key", "authentication_error") {
		return copilotErrorAuth
	}
	if oneOf(code,
		"insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded",
		"project_spend_limit_exceeded", "quota_exhausted", "quota_exceeded", "resource_exhausted",
	) {
		return copilotErrorQuota
	}
	if oneOf(code, "rate_limit_exceeded", "rate_limit_exceeded_error", "slow_down") {
		return copilotErrorRateLimit
	}
	if (useStatus && status == 404) || code == "model_not_supported" || strings.Contains(text, "model is not supported") {
		return copilotErrorNotFound
	}
	if (useStatus && (status == 500 || status == 502 || status == 503 || status == 504)) || strings.Contains(text, "overloaded") {
		return copilotErrorTransient
	}
	return copilotErrorOther
}

func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func copilotErrorCodes(body []byte) []string {
	var value any
	if json.Unmarshal(body, &value) == nil {
		return collectCopilotErrorCodes(value)
	}
	var result []string
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		trimmed := bytes.TrimSpace(line)
		payload, ok := bytes.CutPrefix(trimmed, []byte("data:"))
		if !ok {
			continue
		}
		var event any
		if json.Unmarshal(bytes.TrimSpace(payload), &event) == nil {
			result = append(result, collectCopilotErrorCodes(event)...)
		}
	}
	return result
}

func collectCopilotErrorCodes(value any) []string {
	var output []string
	collectCopilotErrorCodesInto(value, &output)
	return output
}

func collectCopilotErrorCodesInto(value any, output *[]string) {
	switch current := value.(type) {
	case map[string]any:
		for key, nested := range current {
			switch key {
			case "code", "status", "reason", "type":
				switch token := nested.(type) {
				case string:
					pushCopilotErrorCode(output, token)
				case json.Number:
					pushCopilotErrorCode(output, token.String())
				case float64:
					pushCopilotErrorCode(output, strings.TrimSuffix(strings.TrimSuffix(jsonNumberString(token), ".0"), "."))
				default:
					collectCopilotErrorCodesInto(nested, output)
				}
			default:
				collectCopilotErrorCodesInto(nested, output)
			}
		}
	case []any:
		for _, nested := range current {
			collectCopilotErrorCodesInto(nested, output)
		}
	}
}

func pushCopilotErrorCode(output *[]string, value string) {
	token := strings.ToLower(strings.TrimSpace(value))
	if token != "" {
		*output = append(*output, token)
	}
}

func jsonNumberString(value float64) string {
	content, _ := json.Marshal(value)
	return string(content)
}
