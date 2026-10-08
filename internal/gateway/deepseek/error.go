package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
)

func classifyDeepSeekErrorBody(status int, body []byte) providerentity.ErrorClassification {
	var text *string
	if utf8.Valid(body) {
		value := string(body)
		text = &value
	}
	best := providerentity.ErrorClassification{Class: providerentity.ErrorOther}
	if status != http.StatusTooManyRequests {
		best = classifyDeepSeekErrorSignal(&status, nil, text)
	}
	for _, code := range deepSeekErrorCodes(body) {
		var statusValue *int
		var codeText *string
		if status != http.StatusTooManyRequests {
			statusValue = &status
			codeText = &code
		}
		candidate := classifyDeepSeekErrorSignal(statusValue, &code, codeText)
		if deepSeekErrorRank(candidate.Class) < deepSeekErrorRank(best.Class) {
			best = candidate
		}
	}
	return best
}

func classifyDeepSeekErrorSignal(status *int, code, text *string) providerentity.ErrorClassification {
	codeValue := ""
	if code != nil {
		codeValue = strings.ToLower(strings.TrimSpace(*code))
	}
	textValue := ""
	if text != nil {
		textValue = strings.ToLower(*text)
	}
	if status != nil && (*status == http.StatusUnauthorized || *status == http.StatusForbidden) ||
		codeValue == "unauthenticated" || codeValue == "invalid_api_key" || codeValue == "authentication_error" {
		return providerentity.ErrorClassification{Class: providerentity.ErrorAuth}
	}
	switch codeValue {
	case "insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded",
		"project_spend_limit_exceeded", "quota_exhausted", "quota_exceeded", "resource_exhausted":
		return providerentity.ErrorClassification{Class: providerentity.ErrorQuota, Cooldown: 5 * time.Minute}
	case "rate_limit_error", "rate_limit_exceeded", "rate_limit_exceeded_error", "slow_down":
		return providerentity.ErrorClassification{Class: providerentity.ErrorRateLimit, Cooldown: time.Minute}
	}
	if status != nil && *status == http.StatusNotFound || codeValue == "not_found_error" ||
		codeValue == "model_not_supported" || strings.Contains(textValue, "model is not supported") {
		return providerentity.ErrorClassification{Class: providerentity.ErrorNotFound}
	}
	if status != nil && (*status == 500 || *status == 502 || *status == 503 || *status == 504) ||
		codeValue == "overloaded_error" || codeValue == "server_is_overloaded" || strings.Contains(textValue, "overloaded") {
		return providerentity.ErrorClassification{Class: providerentity.ErrorTransient, Cooldown: 10 * time.Second}
	}
	return providerentity.ErrorClassification{Class: providerentity.ErrorOther}
}

func deepSeekErrorCodes(body []byte) []string {
	if value, ok := deepSeekErrorJSON(body); ok {
		var codes []string
		collectDeepSeekErrorCodes(value, &codes)
		return codes
	}
	var codes []string
	for _, line := range strings.Split(string(body), "\n") {
		payload, found := strings.CutPrefix(strings.TrimSpace(line), "data:")
		if !found {
			continue
		}
		if value, ok := deepSeekErrorJSON([]byte(strings.TrimSpace(payload))); ok {
			collectDeepSeekErrorCodes(value, &codes)
		}
	}
	return codes
}

func deepSeekErrorJSON(body []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	var trailing any
	return value, errors.Is(decoder.Decode(&trailing), io.EOF)
}

func collectDeepSeekErrorCodes(value any, codes *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := typed[key]
			if key == "code" || key == "status" || key == "reason" || key == "type" {
				switch token := child.(type) {
				case string:
					appendDeepSeekErrorCode(codes, token)
				case json.Number:
					appendDeepSeekErrorCode(codes, token.String())
				default:
					collectDeepSeekErrorCodes(child, codes)
				}
			} else {
				collectDeepSeekErrorCodes(child, codes)
			}
		}
	case []any:
		for _, child := range typed {
			collectDeepSeekErrorCodes(child, codes)
		}
	}
}

func appendDeepSeekErrorCode(codes *[]string, value string) {
	if code := strings.ToLower(strings.TrimSpace(value)); code != "" {
		*codes = append(*codes, code)
	}
}

func deepSeekErrorRank(class providerentity.ErrorClass) int {
	switch class {
	case providerentity.ErrorAuth:
		return 0
	case providerentity.ErrorQuota:
		return 1
	case providerentity.ErrorRateLimit:
		return 2
	case providerentity.ErrorNotFound:
		return 3
	case providerentity.ErrorTransient:
		return 4
	default:
		return 5
	}
}
