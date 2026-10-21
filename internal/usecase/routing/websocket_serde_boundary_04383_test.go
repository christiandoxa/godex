package routing

import (
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged 0.438.3 parse_runtime_websocket_request_metadata returns an empty
// semantic plan unless serde_json::from_str parses the entire request.
// An unrelated near-f64-maximum number can therefore suppress otherwise
// valid previous_response_id, session, turn, and tool-output affinity.
func TestProdex04383WebSocketSerdeFloatCannotInventHardToolOwner(t *testing.T) {
	for _, tc := range []struct {
		name, literal string
		valid         bool
	}{
		{"serde-invalid-go-finite", `1.7976931348623158e308`, false},
		{"serde-valid-long-mantissa", `1.79769313486231580e308`, true},
		{"serde-valid-previous", `1.7976931348623157e308`, true},
		{"both-overflow", `1.7976931348623159e308`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"type":"response.create","previous_response_id":"resp-foreign",` +
				`"session_id":"session-foreign","x-codex-turn-state":"turn-foreign",` +
				`"input":[{"type":"function_call_output","call_id":"call-1","output":"ok"}],` +
				`"unused":` + tc.literal + `}`
			request := proxymodel.Request{Body: []byte(body), Header: make(http.Header), WebSocketMessage: true}
			got := parseWebSocketRequestMetadata(request)
			if found := got.requiresPreviousResponseAffinity; found != tc.valid {
				t.Fatalf("serde invalid request %s locked owner=%t want=%t metadata=%+v", tc.name, found, tc.valid, got)
			}
			if !tc.valid {
				if got.previousResponseID != "" || got.sessionID != "" || got.turnState != "" {
					t.Fatalf("invalid serde float retained WebSocket continuation metadata %+v", got)
				}
			} else if got.previousResponseID != "resp-foreign" || got.sessionID != "session-foreign" || got.turnState != "turn-foreign" {
				t.Fatalf("valid serde float lost owner metadata %+v", got)
			}
		})
	}
}
