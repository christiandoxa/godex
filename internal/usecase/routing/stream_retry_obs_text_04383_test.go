package routing

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"net/http"
	"testing"
	"time"
)

// Tagged 0.438.3 first inserts every valid http::HeaderValue into HeaderMap,
// then checks HeaderValue::to_str on the *last* case-insensitive matching
// Retry-After header. Bytes >=128 are allowed during HeaderValue::from_str
// but to_str fails: the selected value must SHADOW an earlier valid hint.
func TestProdex04383RetryAfterNonASCIISelectedValueShadowsEarlierHint(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	for _, tc := range []struct {
		name, fields string
		want         time.Duration
	}{
		{"ascii-control", `"Retry-After":"5","retry-after":"4"`, 4 * time.Second},
		{"invalid-ascii-shadow", `"Retry-After":"5","retry-after":"invalid"`, 12 * time.Second},
		{"latin1-shadow", `"Retry-After":"5","retry-after":"é"`, 12 * time.Second},
		{"unicode-space-shadow", `"Retry-After":"5","retry-after":"5\u00a0"`, 12 * time.Second},
		{"unicode-space-single", `"Retry-After":"5\u00a0"`, 12 * time.Second},
		{"unicode-narrow-space-single", `"Retry-After":"\u202f5"`, 12 * time.Second},
		{"raw-utf8-shadow", `"Retry-After":"5","retry-after":"é5"`, 12 * time.Second},
		{"ascii-invalid-control", `"Retry-After":"5","retry-after":"5\n"`, 5 * time.Second},
		{"later-valid-ascii", `"Retry-After":"é","retry-after":"5"`, 5 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Please try again in 12s.","headers":{` + tc.fields + `}}}}`
			got, ok := structuredStreamRetryAdvice([]byte(event), now)
			if !ok || got != tc.want {
				t.Fatalf("duplicate/nonASCII Retry-After candidate gave=%s ok=%t want=%s", got, ok, tc.want)
			}
		})
	}
}

func TestProdex04383NonASCIISelectedRetryHeaderCannotShortenPrecommitCooldown(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	for _, tc := range []struct {
		name, headers string
		want          time.Duration
	}{
		{"valid-duplicate", `"Retry-After":"5","retry-after":"4"`, 4 * time.Second},
		{"unicode-late-value", `"Retry-After":"5","retry-after":"é"`, 12 * time.Second},
		{"unicode-nbsp-value", `"Retry-After":"5","retry-after":"5\u00a0"`, 12 * time.Second},
		{"invalid-ascii-late-value", `"Retry-After":"5","retry-after":"5\n"`, 5 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Please try again in 12s.","headers":{` + tc.headers + `}}}}`
			response := &proxymodel.Response{StatusCode: http.StatusOK,
				PrecommitFailure: &proxymodel.PrecommitFailure{Code: "rate_limit_exceeded", RetryAdviceJSON: []byte(event)}}
			router := &Router{now: func() time.Time { return now }}
			outcome, _, err := router.classifyPrecommitFailure(response, &pendingResponse{response: response})
			if err != nil {
				t.Fatal(err)
			}
			if outcome.kind != responseRetry || outcome.quarantine != tc.want || !outcome.explicitRetryAdvice {
				t.Fatalf("precommit retry cooldown ignored selected duplicate: outcome=%+v want=%s", outcome, tc.want)
			}
		})
	}
}
