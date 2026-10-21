package deepseek

import (
	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"net/http"
	"strings"
	"testing"
)

// Prodex provider_error_codes calls values_from_json_or_sse, which first
// deserializes a *complete* serde_json::Value. A nested, unrelated invalid
// Unicode/number token cannot be ignored while promoting a trusted code.
func TestProdex04384DeepSeekInvalidWholeJSONCannotCreate429RetrySignal(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"canonical", `{"error":{"code":"rate_limit_exceeded"}}`, true},
		{"paired-surrogates", `{"error":{"code":"rate_limit_exceeded"},"unused":"\ud83d\ude00"}`, true},
		{"unpaired-high", `{"error":{"code":"rate_limit_exceeded"},"unused":"\ud800"}`, false},
		{"unpaired-low", `{"error":{"code":"rate_limit_exceeded"},"unused":"\udc00"}`, false},
		{"bad-surrogate-pair", `{"error":{"code":"rate_limit_exceeded"},"unused":"\ud800\u0041"}`, false},
		{"invalid-utf8", `{"error":{"code":"rate_limit_exceeded"},"unused":"` + string([]byte{0xff}) + `"}`, false},
		{"numeric-overflow", `{"error":{"code":"rate_limit_exceeded"},"unused":1e309}`, false},
		{"serde-specific-overflow", `{"error":{"code":"rate_limit_exceeded"},"unused":1.7976931348623158e308}`, false},
		{"near-max-long-mantissa-valid", `{"error":{"code":"rate_limit_exceeded"},"unused":1.79769313486231580e308}`, true},
		{"excessive-depth", `{"error":{"code":"rate_limit_exceeded"},"unused":` + strings.Repeat(`[`, 128) + `0` + strings.Repeat(`]`, 128) + `}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			codes := deepSeekErrorCodes([]byte(tc.body))
			if found := len(codes) > 0; found != tc.valid {
				t.Fatalf("DeepSeek invalid JSON incorrectly yielded codes=%q valid=%t", codes, tc.valid)
			}
			class := classifyDeepSeekErrorBody(http.StatusTooManyRequests, []byte(tc.body)).Class
			want := providerentity.ErrorOther
			if tc.valid {
				want = providerentity.ErrorRateLimit
			}
			if class != want {
				t.Fatalf("DeepSeek JSON validity changed 429 class=%v want=%v", class, want)
			}
		})
	}
}

func TestProdex04384DeepSeekSSEInvalidWholeEventCannotCreate429RetrySignal(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		valid         bool
	}{
		{"canonical", `{"error":{"reason":"rate_limit_error"}}`, true},
		{"invalid-unused-surrogate", `{"error":{"reason":"rate_limit_error"},"unused":"\ud800"}`, false},
		{"invalid-unused-f64", `{"error":{"reason":"rate_limit_error"},"unused":1.7976931348623158e308}`, false},
		{"valid-long-mantissa", `{"error":{"reason":"rate_limit_error"},"unused":1.79769313486231580e308}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "event: error\ndata: " + tc.payload + "\n\n"
			codes := deepSeekErrorCodes([]byte(body))
			if (len(codes) > 0) != tc.valid {
				t.Fatalf("serde-invalid DeepSeek SSE yielded structured codes=%#v", codes)
			}
			actual := classifyDeepSeekErrorBody(http.StatusTooManyRequests, []byte(body))
			want := providerentity.ErrorOther
			if tc.valid {
				want = providerentity.ErrorRateLimit
			}
			if actual.Class != want {
				t.Fatalf("serde-invalid DeepSeek SSE class=%v want=%v", actual.Class, want)
			}
		})
	}
}
