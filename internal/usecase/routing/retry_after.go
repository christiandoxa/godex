package routing

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	defaultRateLimitBackoff = 20 * time.Second
	maxRetryAfter           = 5 * time.Minute
)

func rateLimitCooldown(headers http.Header, body []byte) time.Duration {
	delay, ok := retryAfterMessage(string(body))
	if !ok {
		for _, value := range headers.Values("Retry-After") {
			candidate, found := retryAfterHeader(value)
			if found && (!ok || candidate > delay) {
				delay, ok = candidate, true
			}
		}
	}
	if !ok || delay < defaultRateLimitBackoff {
		return defaultRateLimitBackoff
	}
	return min(delay, maxRetryAfter)
}

// The native 0.438.3 HTTP/SSE error policy folds Retry-After header
// names as ASCII and accepts *only* unsigned integer seconds, capped at
// 300 seconds. Rust runtime_retry_after_from_headers gathers all matching
// values and selects the maximum valid delay; it never parses HTTP dates.
// Keep the older generic retryAfter helper for external provider paths.
func nativeRetryAfterHeaders04383(headers http.Header) time.Duration {
	var greatest time.Duration
	for _, raw := range headers.Values("Retry-After") {
		if delay, ok := retryAfterHeader(raw); ok && delay > greatest {
			greatest = delay
		}
	}
	return greatest
}

func retryAfterHeader(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	// Mojo accepts a positive unsigned UInt64 literal, not +1 or -1.
	seconds, err := strconv.ParseUint(value, 10, 64)
	if err == nil && seconds > 0 {
		if seconds > uint64(maxRetryAfter/time.Second) {
			return maxRetryAfter, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	return 0, false
}

func retryAfterMessage(body string) (time.Duration, bool) {
	marker := "try again in"
	start := retryAfterASCIIFind043714(body, marker)
	if start < 0 {
		return 0, false
	}
	value := strings.TrimLeftFunc(body[start+len(marker):], unicode.IsSpace)
	length := 0
	for length < len(value) && ((value[length] >= '0' && value[length] <= '9') || value[length] == '.') {
		length++
	}
	if length == 0 {
		return 0, false
	}
	numeric := value[:length]
	suffix := strings.TrimSpace(value[length:])
	unit := time.Duration(0)
	if len(suffix) >= 2 && retryAfterASCIIFind043714(suffix[:2], "ms") == 0 {
		unit = time.Millisecond
	} else if len(suffix) >= 1 && retryAfterASCIIFind043714(suffix[:1], "s") == 0 {
		unit = time.Second
	}
	if unit == 0 {
		return 0, false
	}
	return retryAfterTaggedDecimal043714(numeric, unit)
}
