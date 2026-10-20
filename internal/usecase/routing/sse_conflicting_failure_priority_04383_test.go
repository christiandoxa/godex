package routing

import (
	"net/http"
	"testing"
	"time"
)

// In tagged 0.438.3 SSE inspection, runtime_stream_error_policy yields
// quota/rate/overload failure flags independent of the parsed invalid-ID
// marker. Mojo prioritizes quota/rate/overload before the previous-response
// error path. A single error can carry both signals; native SSE must not
// suppress a high-priority quota rotation just because the message looks
// like an invalid previous_response_id.
func TestProdex04383SSEQuotaRateOverloadOutrankInvalidPreviousID(t *testing.T) {
	for _, tc := range []struct {
		name, body                                string
		retry, quota, transient, profile, invalid bool
	}{
		{"quota-and-invalid-id", `{"type":"response.failed","response":{"error":{"code":"insufficient_quota","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}}`, true, true, false, false, false},
		{"rate-and-invalid-id", `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}}`, true, false, true, false, false},
		{"overload-and-invalid-id", `{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}}`, true, false, true, false, false},
		{"profile-and-invalid-id", `{"type":"response.failed","response":{"error":{"code":"deactivated_workspace","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}}`, true, false, false, true, false},
		{"invalid-id-only", `{"type":"response.failed","response":{"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}}`, false, false, false, false, true},
		{"quota-by-reason", `{"type":"response.failed","response":{"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `.","reason":"insufficient_quota"}}}`, true, true, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome, wait := streamOutcome([]byte(tc.body), make(http.Header), time.Unix(1_000_000_000, 0), "openai")
			if wait || (outcome.kind == responseRetry) != tc.retry || outcome.quota != tc.quota || outcome.transient != tc.transient || outcome.profileUnavailable != tc.profile || outcome.invalidPreviousResponseID != tc.invalid {
				t.Fatalf("SSE failure signal precedence wrong: outcome=%+v wait=%v; want retry=%v quota=%v transient=%v profile=%v invalidID=%v", outcome, wait, tc.retry, tc.quota, tc.transient, tc.profile, tc.invalid)
			}
		})
	}
}
