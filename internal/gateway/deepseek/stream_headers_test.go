package deepseek

import (
	"math"
	"net/http"
	"testing"
)

func TestProdex04361DeepSeekSSEHeadersTranslateOnlySupportedRateLimitMetadata(t *testing.T) {
	upstream := http.Header{
		"Content-Type":                   {"text/event-stream"},
		"Content-Length":                 {"999"},
		"X-Synthetic-Upstream":           {"should-not-pass"},
		"Set-Cookie":                     {"sensitive=session"},
		"X-Ratelimit-Limit-Requests":     {"100"},
		"X-Ratelimit-Remaining-Requests": {"75"},
		"X-Ratelimit-Limit-Tokens":       {"200"},
		"X-Ratelimit-Remaining-Tokens":   {"0"},
	}
	headers := deepSeekSSEHeaders(upstream)
	for key, want := range map[string]string{
		"Content-Type":                             "text/event-stream; charset=utf-8",
		"X-Deepseek-Requests-Limit-Name":           "DeepSeek requests",
		"X-Deepseek-Requests-Primary-Used-Percent": "25",
		"X-Deepseek-Tokens-Limit-Name":             "DeepSeek tokens",
		"X-Deepseek-Tokens-Primary-Used-Percent":   "100",
	} {
		if got := headers.Get(key); got != want {
			t.Fatalf("%s=%q, want %q", key, got, want)
		}
	}
	for _, key := range []string{"Content-Length", "X-Synthetic-Upstream", "Set-Cookie", "X-Ratelimit-Limit-Requests"} {
		if got := headers.Get(key); got != "" {
			t.Fatalf("untrusted upstream SSE header %s leaked: %q", key, got)
		}
	}
}
func TestProdex04361DeepSeekSSEHeaderRejectsInvalidQuotaFractions(t *testing.T) {
	for _, test := range []struct{ limit, remaining string }{
		{"0", "1"}, {"NaN", "1"}, {"+Inf", "1"}, {"10", "NaN"}, {"bad", "4"},
		{"10", "missing"},
	} {
		headers := deepSeekSSEHeaders(http.Header{
			"X-Ratelimit-Limit-Requests":     []string{test.limit},
			"X-Ratelimit-Remaining-Requests": []string{test.remaining},
		})
		if got := headers.Get("X-Deepseek-Requests-Primary-Used-Percent"); got != "" {
			t.Fatalf("invalid upstream quota ratio %.8v accepted: %s", test, got)
		}
	}
	value, ok := deepSeekHeaderNumber("42")
	if !ok || math.Abs(value-42) > 0 {
		t.Fatal("numeric header rejected")
	}
}
