package routing

import (
	"io"
	"net/http"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"testing"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
)

// Tagged runtime_http_error_policy uses the compiled raw JSON scanner,
// capped at 32 nested levels and 65536 bytes. It does not use serde_json
// floating-point range validation, and its code keys have exact ASCII names.
func TestProdex04383HTTP429MojoJSONCodeAndBoundaryPolicy(t *testing.T) {
	quota := providerentity.ErrorQuota
	rate := providerentity.ErrorRateLimit
	other := providerentity.ErrorOther
	deep := strings.Repeat(`{"wrapper":`, 34) + `{"code":"insufficient_quota"}` + strings.Repeat(`}`, 34)
	tooLong := `{"error":{"code":"insufficient_quota"},"padding":"` + strings.Repeat("x", 65536) + `"}`
	for _, tc := range []struct {
		name, payload string
		want          providerentity.ErrorClass
	}{
		{"quota", `{"error":{"code":"insufficient_quota"}}`, quota},
		{"uppercase-code-value", `{"error":{"code":"INSUFFICIENT_QUOTA"}}`, quota},
		{"space-padded-code-value", `{"error":{"code":"insufficient_quota "}}`, rate},
		{"space-padded-code-key", `{"error":{" code ":"insufficient_quota"}}`, rate},
		{"uppercase-code-key", `{"error":{"CODE":"insufficient_quota"}}`, rate},
		{"code-not-enough", `{"error":{"message":"insufficient_quota"}}`, rate},
		{"numeric-overflow-unrelated", `{"error":{"code":"insufficient_quota"},"unrelated":1e309}`, quota},
		{"numeric-underflow-unrelated", `{"error":{"code":"insufficient_quota"},"unrelated":1e-99999}`, quota},
		{"escaped-code-value", `{"error":{"code":"insufficient_\\u0071uota"}}`, rate},
		{"escaped-unicode-code-value", `{"error":{"code":"insufficient_\u0071uota"}}`, rate},
		{"escaped-code-key", `{"error":{"\u0063ode":"insufficient_quota"}}`, rate},
		{"duplicated-code-first-quota-then-rate", `{"error":{"code":"insufficient_quota","code":"rate_limit_exceeded"}}`, rate},
		{"duplicated-code-first-rate-then-quota", `{"error":{"code":"rate_limit_exceeded","code":"insufficient_quota"}}`, rate},
		{"deep-json", deep, rate},
		{"over-65536", tooLong, other},
		{"invalid-utf8", `{"error":{"code":"insufficient_quota"},"note":"` + string([]byte{0xff}) + `"}`, rate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := openAI429Classification([]byte(tc.payload)); got.Class != tc.want {
				t.Fatalf("HTTP 429 class=%v want=%v for %s", got.Class, tc.want, tc.name)
			}
		})
	}
}

// Classifier parity must affect the real HTTP 429 recovery policy, not just
// a helper return value. A source-tagged quota response marks the account as
// quota exhausted; a generic 429 rate event sets transient retry; malformed
// oversized/invalid-UTF8 bodies leave the provider response untouched.
func TestProdex04383HTTP429MojoPolicyRoutesCorrectRecoveryKind(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, tc := range []struct {
		name, body       string
		wantKind         responseKind
		quota, transient bool
	}{
		{"canonical-quota", `{"error":{"code":"insufficient_quota"}}`, responseRetry, true, false},
		{"numeric-overflow-keeps-quota", `{"error":{"code":"insufficient_quota"},"debug":1e309}`, responseRetry, true, false},
		{"padded-code-rate", `{"error":{"code":"insufficient_quota "}}`, responseRetry, false, true},
		{"escaped-key-rate", `{"error":{"\\u0063ode":"insufficient_quota"}}`, responseRetry, false, true},
		{"valid-rate", `{"error":{"code":"rate_limit_exceeded"}}`, responseRetry, false, true},
		{"overdepth-rate", strings.Repeat(`{"x":`, 34) + `{"code":"insufficient_quota"}` + strings.Repeat(`}`, 34), responseRetry, false, true},
		{"oversized-pass-through", `{"error":{"code":"insufficient_quota"},"padding":"` + strings.Repeat("x", 65536) + `"}`, responsePass, false, false},
		{"invalid-utf8-rate-fallback", `{"error":{"code":"insufficient_quota"},"text":"` + string([]byte{0xff}) + `"}`, responseRetry, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := &Router{now: func() time.Time { return now }, maxInspect: 1 << 20}
			response := &proxymodel.Response{StatusCode: http.StatusTooManyRequests,
				Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}
			outcome, pending, err := router.classify(response, "openai")
			if pending != nil {
				defer pending.close()
			}
			if err != nil {
				t.Fatal(err)
			}
			if outcome.kind != tc.wantKind || outcome.quota != tc.quota || outcome.transient != tc.transient {
				t.Fatalf("HTTP 429 outcome=%+v want kind=%d quota=%v transient=%v", outcome, tc.wantKind, tc.quota, tc.transient)
			}
		})
	}
}
