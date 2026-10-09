package routing

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxStreamRetryAdviceBytes = 64 << 10

// structuredStreamRetryAdvice implements the tagged Prodex 0.437.0 precommit
// header precedence for SSE and WebSocket error events. The JSON object is
// bounded; malformed/non-retryable events cannot influence the retry policy.
func structuredStreamRetryAdvice(data []byte, now time.Time) (time.Duration, bool) {
	if len(data) == 0 || len(data) > maxStreamRetryAdviceBytes {
		return 0, false
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil || root == nil {
		return 0, false
	}
	var eventType string
	if json.Unmarshal(root["type"], &eventType) != nil {
		return 0, false
	}
	var primary, secondary json.RawMessage
	var errorObject map[string]json.RawMessage
	switch eventType {
	case "response.failed":
		var response map[string]json.RawMessage
		if json.Unmarshal(root["response"], &response) != nil {
			return 0, false
		}
		if json.Unmarshal(response["error"], &errorObject) != nil {
			return 0, false
		}
		primary = errorObject["headers"]
	case "error":
		if json.Unmarshal(root["error"], &errorObject) != nil {
			return 0, false
		}
		primary = errorObject["headers"]
		secondary = root["headers"]
	default:
		return 0, false
	}
	var message string
	if errorObject != nil {
		_ = json.Unmarshal(errorObject["message"], &message)
	}
	fallback, haveFallback := retryAfterMessage(message)
	if delay, ok := streamRetryAfterHeaders(primary, now); ok {
		return delay, true
	}
	if delay, ok := streamRetryAfterHeaders(secondary, now); ok {
		return delay, true
	}
	return fallback, haveFallback
}

// An invalid *selected* header replaces an earlier duplicate, as the Rust
// header insertion logic does. Parsing a case-insensitive Go map in random
// iteration order would disagree with serde_json's sorted map iteration.
func streamRetryAfterHeaders(raw json.RawMessage, now time.Time) (time.Duration, bool) {
	var headers map[string]json.RawMessage
	if json.Unmarshal(raw, &headers) != nil || headers == nil {
		return 0, false
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	selected := ""
	found := false
	for _, key := range keys {
		if !strings.EqualFold(key, "retry-after") {
			continue
		}
		var value string
		if err := json.Unmarshal(headers[key], &value); err != nil {
			switch string(headers[key]) {
			case "true", "false":
				value = string(headers[key])
			default:
				// JSON numbers are accepted as header values.
				var numeric json.Number
				if json.Unmarshal(headers[key], &numeric) != nil {
					continue
				}
				value = numeric.String()
			}
		}
		if !validStreamRetryHeaderValue(value) {
			continue
		}
		selected = value
		found = true
	}
	if !found {
		return 0, false
	}
	return parseStreamRetryHeader(selected, now)
}

func validStreamRetryHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c != 9 && (c < 32 || c >= 127) {
			return false
		}
	}
	return true
}

func parseStreamRetryHeader(raw string, now time.Time) (time.Duration, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}
	allDigits := true
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err != nil || seconds > uint64(maxRetryAfter/time.Second) {
			return maxRetryAfter, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	date, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if !date.After(now) {
		return 0, true
	}
	remaining := date.Sub(now)
	if remaining >= maxRetryAfter {
		return maxRetryAfter, true
	}
	nanos := remaining.Nanoseconds()
	return time.Duration((nanos+999_999)/1_000_000) * time.Millisecond, true
}
