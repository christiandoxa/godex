package deepseek

import (
	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"net/http"
	"testing"
	"time"
)

// Tagged 0.438.4 provider_error.mojo lowercases A-Z ASCII bytes only.
// Unicode U+212A Kelvin sign and U+0130 dotted capital I must never
// impersonate ASCII k/i in authentication, quota or rate-limit codes.
func TestProdex04384DeepSeekErrorCodesUseASCIIOnlyCaseFolding(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		class      providerentity.ErrorClass
		cooldown   time.Duration
	}{
		{"canonical-auth", `{"error":{"code":"INVALID_API_KEY"}}`, providerentity.ErrorAuth, 0},
		{"unicode-kelvin-auth", `{"error":{"code":"invalid_api_Key"}}`, providerentity.ErrorOther, 0},
		{"canonical-rate", `{"error":{"code":"RATE_LIMIT_ERROR"}}`, providerentity.ErrorRateLimit, time.Minute},
		{"unicode-dotted-I-rate", `{"error":{"code":"rate_lİmit_error"}}`, providerentity.ErrorOther, 0},
		{"canonical-quota", `{"error":{"code":"INSUFFICIENT_QUOTA"}}`, providerentity.ErrorQuota, 5 * time.Minute},
		{"unicode-dotted-I-quota", `{"error":{"code":"İNSUFFİCİENT_QUOTA"}}`, providerentity.ErrorOther, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyDeepSeekErrorBody(http.StatusTooManyRequests, []byte(tc.body))
			if got.Class != tc.class || got.Cooldown != tc.cooldown {
				t.Fatalf("unicode code %s class=%v cool=%s want=%v/%s", tc.body, got.Class, got.Cooldown, tc.class, tc.cooldown)
			}
		})
	}
}

func TestProdex04384DeepSeekErrorTextCannotForgeKnownMessage(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		class      providerentity.ErrorClass
	}{
		{"canonical-model", `{"error":{"message":"MODEL IS NOT SUPPORTED"}}`, providerentity.ErrorNotFound},
		{"dotted-I-model", `{"error":{"message":"MODEL İS NOT SUPPORTED"}}`, providerentity.ErrorOther},
		{"mixed-ASCII-model", `{"error":{"message":"Model Is Not Supported"}}`, providerentity.ErrorNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyDeepSeekErrorBody(http.StatusBadRequest, []byte(tc.text))
			if got.Class != tc.class {
				t.Fatalf("Unicode message %q class=%v want=%v", tc.text, got.Class, tc.class)
			}
		})
	}
}

func TestProdex04384DeepSeekErrorTokenExtractionPreservesNonASCIICase(t *testing.T) {
	body := []byte(`{"code":"THÉ_雪","reason":"RATE_LİMIT_ERROR","error":{"code":"RESOURCE_EXHAUSTED"}}`)
	want := []string{"thÉ_雪", "resource_exhausted", "rate_lİmit_error"}
	codes := deepSeekErrorCodes(body)
	if len(codes) != len(want) {
		t.Fatalf("provider token count %#v want %#v", codes, want)
	}
	// The canonical serde tree traversal is key sorted: code, error, reason.
	for i := range codes {
		if codes[i] != want[i] {
			t.Fatalf("provider token %d=%q want %q all=%v", i, codes[i], want[i], codes)
		}
	}
}
