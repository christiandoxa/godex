package routing

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
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

func retryAfterHeader(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err == nil && seconds > 0 {
		if seconds > int64(maxRetryAfter/time.Second) {
			return maxRetryAfter, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	return 0, false
}

func retryAfterMessage(body string) (time.Duration, bool) {
	message := strings.ToLower(body)
	start := strings.Index(message, "try again in")
	if start < 0 {
		return 0, false
	}
	value := strings.TrimSpace(message[start+len("try again in"):])
	length := 0
	for length < len(value) && ((value[length] >= '0' && value[length] <= '9') || value[length] == '.') {
		length++
	}
	if length == 0 {
		return 0, false
	}
	number, err := strconv.ParseFloat(value[:length], 64)
	if err != nil || number < 0 {
		return 0, false
	}
	suffix := strings.TrimSpace(value[length:])
	unit := time.Second
	switch {
	case strings.HasPrefix(suffix, "ms"):
		unit = time.Millisecond
	case strings.HasPrefix(suffix, "s"), strings.HasPrefix(suffix, "second"):
	default:
		return 0, false
	}
	if number >= float64(maxRetryAfter/unit) {
		return maxRetryAfter, true
	}
	milliseconds := math.Ceil(number * float64(unit) / float64(time.Millisecond))
	return time.Duration(milliseconds) * time.Millisecond, true
}
