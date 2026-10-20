package routing

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// RuntimeMojo RUNTIME_ERROR_MODE_STREAM admits code/type/status/reason;
// generic provider error_code, scalar error and arbitrary message fields
// cannot create a precommit rotation on native OpenAI Responses SSE.
func TestProdex04383SSEPrecommitOnlyCanonicalCodeFieldsRotate(t *testing.T) {
	for _, tc := range []struct {
		name, payload                     string
		rotate, quota, profile, transient bool
	}{
		{"quota-code", `{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}}}`, true, true, false, false},
		{"quota-reason", `{"type":"response.failed","response":{"error":{"reason":"insufficient_quota"}}}`, true, true, false, false},
		{"quota-status", `{"type":"response.failed","response":{"error":{"status":"insufficient_quota"}}}`, true, true, false, false},
		{"quota-type", `{"type":"response.failed","response":{"error":{"type":"insufficient_quota"}}}`, true, true, false, false},
		{"quota-scalar-error", `{"type":"response.failed","response":{"error":"insufficient_quota"}}`, false, false, false, false},
		{"quota-error-code-alias", `{"type":"response.failed","response":{"error":{"error_code":"insufficient_quota"}}}`, false, false, false, false},
		{"rate-error-code-alias", `{"type":"response.failed","response":{"error":{"error_code":"rate_limit_exceeded"}}}`, false, false, false, false},
		{"rate-code", `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded"}}}`, true, false, false, true},
		{"rate-status", `{"type":"response.failed","response":{"error":{"status":"slow_down"}}}`, true, false, false, true},
		{"profile-code", `{"type":"response.failed","response":{"error":{"code":"deactivated_workspace"}}}`, true, false, true, false},
		{"profile-note-not-code", `{"type":"response.failed","response":{"error":{"note":"deactivated_workspace"}}}`, false, false, false, false},
		{"profile-message-not-code", `{"type":"response.failed","response":{"error":{"message":"deactivated_workspace"}}}`, false, false, false, false},
		{"overload-code", `{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`, true, false, false, true},
		{"quota-kelvin-code", `{"type":"response.failed","response":{"error":{"code":"worKspace_member_credits_depleted"}}}`, false, false, false, false},
		{"rate-unicode-key", `{"type":"response.failed","response":{"error":{"error_cōde":"rate_limit_exceeded"}}}`, false, false, false, false},
		{"rate-over-depth", strings.Repeat(`{"wrap":`, 34) + `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded"}}}` + strings.Repeat(`}`, 34), false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome, wait := streamOutcome([]byte(tc.payload), make(http.Header), time.Unix(1000000000, 0), "openai")
			if wait || (outcome.kind == responseRetry) != tc.rotate || outcome.quota != tc.quota || outcome.profileUnavailable != tc.profile || outcome.transient != tc.transient {
				t.Fatalf("native stream %s unexpectedly classified: %+v wait=%t want rotate=%t quota=%t profile=%t transient=%t", tc.name, outcome, wait, tc.rotate, tc.quota, tc.profile, tc.transient)
			}
		})
	}
}
