package routing

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Tagged runtime_stream_error_policy uses serde_json::from_slice for the
// entire event before selecting JSON retry-after headers. A valid header is
// not actionable if an unrelated field would make serde_json reject it.
func TestProdex04383RetryAdviceRequiresCompleteSerdeJSON(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	base := `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","headers":{"Retry-After":"5"}}}`
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"canonical", base + `}`, true},
		{"valid-surrogate-pair", base + `,"unused":"\ud83d\ude00"}`, true},
		{"valid-escaped-slash", base + `,"unused":"\\ud800"}`, true},
		{"finite-exponent", base + `,"unused":1e308}`, true},
		{"malformed-high-surrogate", base + `,"unused":"\ud800"}`, false},
		{"malformed-low-surrogate", base + `,"unused":"\udc00"}`, false},
		{"malformed-utf8", base + `,"unused":"` + string([]byte{0xff}) + `"}`, false},
		{"overflow-number", base + `,"unused":1e309}`, false},
		{"excessive-depth", base + `,"unused":` + strings.Repeat(`{"entry":`, 128) + `0` + strings.Repeat(`}`, 128) + `}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			delay, ok := structuredStreamRetryAdvice([]byte(tc.body), now)
			if ok != tc.valid || (ok && delay != 5*time.Second) {
				t.Fatalf("retry advice from malformed SSE: delay=%s exists=%t want=%t", delay, ok, tc.valid)
			}
		})
	}
}

// Rust http::HeaderName is ASCII-only. A Unicode case-folding confusable in
// a JSON header key cannot claim an unrelated Retry-After field.
func TestProdex04383RetryAdviceHeaderNamesAreASCIIOnly(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	for _, tc := range []struct {
		name, fields string
		want         time.Duration
		valid        bool
	}{
		{"canonical", `"Retry-After":"5"`, 5 * time.Second, true},
		{"uppercase", `"RETRY-AFTER":"6"`, 6 * time.Second, true},
		{"unicode-long-s", `"retry-aſter":"7"`, 0, false},
		{"confusable-plus-valid", `"retry-aſter":"7","retry-after":"4"`, 4 * time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","headers":{` + tc.fields + `}}}}`
			delay, ok := structuredStreamRetryAdvice([]byte(body), now)
			if ok != tc.valid || (ok && delay != tc.want) {
				t.Fatalf("confusable Retry-After header influenced policy: got=%s ok=%t want=%s valid=%t", delay, ok, tc.want, tc.valid)
			}
		})
	}
}

// WebSocket and transport adapters may surface an already inspected
// PrecommitFailure with RetryAdviceJSON directly; the ordinary SSE inspector
// does not run on this path. Malformed JSON must not change rate backoff.
func TestProdex04383MalformedPrecommitJSONCannotOverrideRateCooldown(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		valid      bool
	}{
		{"valid", `"ok"`, true},
		{"valid-surrogate-pair", `"\ud83d\ude00"`, true},
		{"invalid-surrogate", `"\ud800"`, false},
		{"invalid-overflow", `1e309`, false},
		{"invalid-utf8", `"bad-` + string([]byte{0xff}) + `"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","headers":{"Retry-After":"5"}}},"note":` + tc.note + `}`
			response := &proxymodel.Response{StatusCode: 200, PrecommitFailure: &proxymodel.PrecommitFailure{
				Code: "rate_limit_exceeded", RetryAdviceJSON: []byte(payload),
			}}
			router := &Router{now: func() time.Time { return time.Unix(1_000_000_000, 0) }}
			outcome, _, err := router.classifyPrecommitFailure(response, &pendingResponse{response: response})
			if err != nil {
				t.Fatal(err)
			}
			if outcome.kind != responseRetry || !outcome.transient {
				t.Fatalf("untrusted error JSON changed primary retry class: %+v", outcome)
			}
			if outcome.explicitRetryAdvice != tc.valid || (tc.valid && outcome.quarantine != 5*time.Second) ||
				(!tc.valid && outcome.quarantine == 5*time.Second) {
				t.Fatalf("malformed precommit JSON set retry advice: %+v valid=%t", outcome, tc.valid)
			}
		})
	}
}

// Bound-turn overload planning also consumes buffered SSE retry advice. A
// malformed response.failed event is not a valid retry-after source even if
// the selected token would otherwise be a well-formed header.
func TestProdex04383MalformedSSECannotCreateBoundOverloadRetryAdvice(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	for _, tc := range []struct {
		name, note string
		valid      bool
	}{
		{"valid", `"ok"`, true},
		{"invalid-surrogate", `"\ud800"`, false},
		{"invalid-numeric", `1e309`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","headers":{"Retry-After":"5"}}},"note":` + tc.note + `}`
			response := &proxymodel.Response{Header: make(http.Header)}
			pending := &pendingResponse{prefix: []byte("data: " + event + "\n\n")}
			delay, ok := boundOverloadAdvice(response, pending, now)
			if ok != tc.valid || (ok && delay != 5*time.Second) {
				t.Fatalf("malformed SSE changed overload retry advice: delay=%s ok=%t wantValid=%t", delay, ok, tc.valid)
			}
		})
	}
}
