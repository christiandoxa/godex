package routing

import (
	"net/http"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// serde_json 1.0.151 (without float_roundtrip) can reject a number that
// strconv.ParseFloat considers finite because serde's parsed u64
// significand is multiplied by a rounded pow10 f64. This is a whole-JSON
// validity boundary, not just a difference in canonical printed decimals.
func TestProdex04383SerdeF64MaxBoundaryControlsWholeJSONValidity(t *testing.T) {
	for _, tc := range []struct {
		name, literal string
		valid         bool
	}{
		{"serde-finite", `1.7976931348623157e308`, true},
		{"serde-finite-rounddown", `1.7976931348623156e308`, true},
		{"serde-rejects-go-finite", `1.7976931348623158e308`, false},
		{"both-reject", `1.7976931348623159e308`, false},
		{"long-mantissa-rounddown", `1.79769313486231580e308`, true},
		{"long-positive-integer-exp", `17.976931348623157e307`, true},
		{"zero-overflow-exponent", `0e9999999999999999999999`, true},
		{"tiny-negative-exponent", `1e-9999999999`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"type":"response.completed","usage":{"cached_input_tokens":91},"ignored":` + tc.literal + `}`)
			_, valid := decodeNativeUsageJSON(payload)
			if valid != tc.valid {
				t.Fatalf("whole JSON serde validity for %s = %t want %t", tc.literal, valid, tc.valid)
			}
			usage, found := terminalTokenUsage(payload)
			if found != tc.valid || (found && usage.CachedInputTokens != 91) {
				t.Fatalf("invalid serde float supplied native usage: %+v found=%t want=%t", usage, found, tc.valid)
			}
		})
	}
}

// The same whole-document validity is required before permitting an
// invalid-previous-response owner to resume with a reconstructed history.
func TestProdex04383SerdeF64MaxBoundaryCannotAuthorizeHTTPReplay(t *testing.T) {
	for _, tc := range []struct {
		name, ignored string
		allow         bool
	}{
		{"valid-max", `1.7976931348623157e308`, true},
		{"invalid-serde-finite-go", `1.7976931348623158e308`, false},
		{"valid-truncated-mantissa", `1.79769313486231580e308`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header),
				Body: []byte(`{"model":"gpt-6","previous_response_id":"resp-dead","session_id":"verified-session",` +
					`"input":[{"role":"user"},{"role":"assistant"},{"role":"user"}],"ignored":` + tc.ignored + `}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			}
			_, ok := fullHistoryRecoveryRequest(req)
			if ok != tc.allow {
				t.Fatalf("invalid serde f64 boundary authorized replay=%t want=%t", ok, tc.allow)
			}
		})
	}
}

// A malformed structured SSE rate-limit signal cannot spend an alternate
// account retry or adopt a bogus Retry-After header simply because Go's
// decimal parser would round the unrelated high float down to finite.
func TestProdex04383SerdeF64OverflowCannotMintSSEPrecommitRateAdvice(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	for _, tc := range []struct {
		name, ignored string
		valid         bool
	}{
		{"serde-finite-max", `1.7976931348623157e308`, true},
		{"serde-overflow", `1.7976931348623158e308`, false},
		{"serde-long-mantissa-finite", `1.79769313486231580e308`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded",` +
				`"headers":{"Retry-After":"5"}}},"unused":` + tc.ignored + `}`)
			outcome, wait := streamOutcome(payload, make(http.Header), now, "openai")
			if tc.valid {
				if wait || outcome.kind != responseRetry {
					t.Fatalf("valid serde float suppressed rate retry: %+v wait=%t", outcome, wait)
				}
			} else if wait || outcome.kind != responsePass {
				t.Fatalf("serde float overflow caused false rate retry: %+v wait=%t", outcome, wait)
			}
			delay, ok := structuredStreamRetryAdvice(payload, now)
			if ok != tc.valid || (ok && delay != 5*time.Second) {
				t.Fatalf("serde float overflow yielded retry-after=%s ok=%t want=%t", delay, ok, tc.valid)
			}
		})
	}
}

// Parsing the body as an independent Go map must not permit an invalid
// whole-JSON previous_response_id to claim another verified account. The
// same rule applies to session hints inside x-codex-turn-metadata.
func TestProdex04383SerdeF64OverflowCannotStealRequestContinuation(t *testing.T) {
	for _, tc := range []struct {
		name, ignored string
		valid         bool
	}{
		{"invalid-previous", `1.7976931348623158e308`, false},
		{"valid-previous", `1.79769313486231580e308`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := proxymodel.Request{
				Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header),
				Body:           []byte(`{"model":"gpt-6","previous_response_id":"resp-foreign-owner","input":[],"unused":` + tc.ignored + `}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			}
			keys := requestAffinityForRoute(request, request.Body, quotamodel.RouteKindResponses)
			if found := keys.previous == "resp-foreign-owner"; found != tc.valid {
				t.Fatalf("JSON %s borrowed foreign previous response: keys=%+v wantValid=%t", tc.name, keys, tc.valid)
			}
			header := `{"session_id":"foreign-session","ignored":` + tc.ignored + `}`
			request.Header.Set("x-codex-turn-metadata", header)
			request.Body = []byte(`{"model":"gpt-6","input":[]}`)
			keys = requestAffinityForRoute(request, request.Body, quotamodel.RouteKindResponses)
			if found := keys.session == "foreign-session"; found != tc.valid {
				t.Fatalf("JSON %s borrowed foreign session from invalid metadata header: keys=%+v wantValid=%t", tc.name, keys, tc.valid)
			}
		})
	}
}
