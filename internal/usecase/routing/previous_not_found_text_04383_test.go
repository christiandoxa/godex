package routing

import (
	"strings"
	"testing"
)

// Prodex runtime_proxy's extracted previous-response message first visits
// parsed serde_json objects, then falls back to tagged Mojo text classifiers
// only when structured parsing yields no previous-response evidence.
func TestProdex04383PreviousResponseNotFoundAcceptsTextAndRejectsMalformedJSON(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"structured-code", `{"error":{"code":"previous_response_not_found"}}`, true},
		{"nested-code", `{"response":{"error":{"code":"previous_response_not_found"}}}`, true},
		{"wrong-code", `{"error":{"code":"insufficient_quota"}}`, false},
		{"text-code", `previous_response_not_found`, true},
		{"text-code-prefix", `previous_response_not_found: chain missing`, true},
		{"text-no-found", `previous response chain not found on selected account`, true},
		{"text-invalid-id", "Invalid `previous_response_id`.", true},
		{"text-tool-error", `invalid_request_error: no tool call found for call_id`, true},
		{"text-tool-short", `No tool call found for call_id`, true},
		{"text-no-function", `no function call found`, true},
		{"text-unrelated", `server is overloaded`, false},
		{"structured-malformed-surrogate", `{"error":{"code":"previous_response_not_found"},"unused":"\ud800"}`, false},
		{"structured-malformed-overflow", `{"error":{"code":"previous_response_not_found"},"unused":1e309}`, false},
		{"structured-malformed-depth", `{"error":{"code":"previous_response_not_found"},"unused":` + strings.Repeat(`{"a":`, 128) + `0` + strings.Repeat(`}`, 128) + `}`, false},
		{"structured-malformed-utf8", `{"error":{"code":"previous_response_not_found"},"unused":"` + string([]byte{0xff}) + `"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := previousResponseNotFound([]byte(tc.body)); got != tc.want {
				t.Fatalf("previous-response error detected=%t want=%t body=%q", got, tc.want, tc.body)
			}
		})
	}
}
