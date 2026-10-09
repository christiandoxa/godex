package deepseek

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DeepSeek's Responses SSE adapter is a new representation. Do not expose
// arbitrary upstream headers or framing, which the tagged Prodex 0.436.1
// stream bridge likewise does not forward. Map only Codex rate-limit
// metadata supported by the reference bridge.
func deepSeekSSEHeaders(upstream http.Header) http.Header {
	headers := make(http.Header)
	headers.Set("Content-Type", "text/event-stream; charset=utf-8")
	for _, bucket := range []string{"requests", "tokens"} {
		limit, ok := deepSeekHeaderNumber(upstream.Get("X-Ratelimit-Limit-" + bucket))
		if !ok || limit <= 0 {
			continue
		}
		remaining, ok := deepSeekHeaderNumber(upstream.Get("X-Ratelimit-Remaining-" + bucket))
		if !ok {
			continue
		}
		used := math.Min(100, math.Max(0, (limit-remaining)/limit*100))
		used = math.Round(used*10) / 10
		base := "X-Deepseek-" + strings.ToUpper(bucket[:1]) + bucket[1:]
		headers.Set(base+"-Limit-Name", "DeepSeek "+bucket)
		headers.Set(base+"-Primary-Used-Percent", strconv.FormatFloat(used, 'f', -1, 64))
		if reset, ok := deepSeekResetEpoch(upstream.Get("X-Ratelimit-Reset-"+bucket), time.Now()); ok {
			headers.Set(base+"-Primary-Reset-At", strconv.FormatInt(reset, 10))
		}
	}
	return headers
}

func deepSeekHeaderNumber(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

// Prodex supports absolute RFC3339/Unix values and provider-relative
// reset durations. Invalid values must not be copied into Codex headers.
func deepSeekResetEpoch(value string, now time.Time) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if when, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return when.Unix(), true
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		if n > 10_000_000_000 {
			return n / 1000, true
		}
		if n > now.Unix()-60 {
			return n, true
		}
		if n < 0 {
			n = 0
		}
		return now.Unix() + n, true
	}
	if d, err := time.ParseDuration(value); err == nil {
		if d < 0 {
			d = 0
		}
		return now.Add(d).Unix(), true
	}
	return 0, false
}
