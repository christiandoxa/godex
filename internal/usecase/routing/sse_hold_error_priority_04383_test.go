package routing

import (
	"net/http"
	"testing"
	"time"
)

// The tagged 0.438.3 SSE inspection step evaluates quota/rate/overload and
// previous-response failure flags BEFORE considering the hold-event kind.
// A response.created/keepalive envelope is not permission to hide a matching
// error code from the precommit classifier.
func TestProdex04383SSEFailureSignalsOutrankHoldEventKinds(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	for _, tc := range []struct {
		name, payload                                               string
		wantRetry, wantQuota, wantTransient, wantWait, wantPrevious bool
	}{
		{"created-quota", `{"type":"response.created","response":{"error":{"code":"insufficient_quota"}}}`, true, true, false, false, false},
		{"inprogress-rate", `{"type":"response.in_progress","response":{"error":{"code":"rate_limit_exceeded"}}}`, true, false, true, false, false},
		{"keepalive-quota", `{"type":"keepalive","error":{"code":"insufficient_quota"}}`, true, true, false, false, false},
		{"metadata-overload", `{"type":"response.metadata","error":{"code":"server_is_overloaded"}}`, true, false, true, false, false},
		{"created-previous", `{"type":"response.created","error":{"code":"previous_response_not_found"}}`, false, false, false, false, true},
		{"created-no-error", `{"type":"response.created","response":{"id":"resp-new"}}`, false, false, false, true, false},
		{"keepalive-no-error", `{"type":"keepalive"}`, false, false, false, true, false},
		{"inprogress-error-alias", `{"type":"response.in_progress","error":{"error_code":"rate_limit_exceeded"}}`, false, false, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome, wait := streamOutcome([]byte(tc.payload), make(http.Header), now, "openai")
			if (outcome.kind == responseRetry) != tc.wantRetry || outcome.quota != tc.wantQuota || outcome.transient != tc.wantTransient || wait != tc.wantWait || outcome.previousResponseNotFound != tc.wantPrevious {
				t.Fatalf("native SSE hold/error priority mismatch: %+v wait=%t expected retry=%t quota=%t transient=%t wait=%t previous=%t", outcome, wait, tc.wantRetry, tc.wantQuota, tc.wantTransient, tc.wantWait, tc.wantPrevious)
			}
		})
	}
}
