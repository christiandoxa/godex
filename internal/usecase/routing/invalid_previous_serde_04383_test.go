package routing

import (
	"net/http"
	"strings"
	"testing"
)

// The tagged runtime_proxy_body_is_invalid_previous_response_id parses the
// entire body with serde_json::from_slice. An otherwise matching error tag
// cannot release a hard previous owner if an unrelated JSON field is invalid.
func TestProdex04383InvalidPreviousResponseDetectionRequiresValidSerdeJSON(t *testing.T) {
	prefix := `{"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}`
	valid := prefix + `}`
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"canonical-control", valid, true},
		{"valid-unicode", prefix + `,"unused":"\ud83d\ude00"}`, true},
		{"valid-numeric", prefix + `,"unused":1e308}`, true},
		{"raw-utf8", prefix + `,"unused":"` + string([]byte{0xff}) + `"}`, false},
		{"unpaired-high", prefix + `,"unused":"\ud800"}`, false},
		{"unpaired-low", prefix + `,"unused":"\udc00"}`, false},
		{"overflow", prefix + `,"unused":1e309}`, false},
		{"deep", prefix + `,"unused":` + strings.Repeat(`{"item":`, 128) + `0` + strings.Repeat(`}`, 128) + `}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := invalidPreviousResponseID([]byte(tc.body)); got != tc.want {
				t.Fatalf("invalid previous error JSON classified=%t want=%t", got, tc.want)
			}
		})
	}
}

// A broken 400 error envelope must remain opaque; the successfully parsed
// canonical error continues to admit the established owned-history recovery.
func TestProdex04383MalformedInvalidPreviousErrorCannotReplay(t *testing.T) {
	for _, tc := range []struct {
		name, suffix string
		wantAttempts int
	}{
		{"invalid-surrogate", `,"unused":"\ud800"}`, 1},
		{"overflow", `,"unused":1e309}`, 1},
		{"valid", `}`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}` + tc.suffix
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
				{http.StatusBadRequest, "application/json", body},
				{http.StatusOK, "application/json", `{"id":"resp-recovered","object":"response"}`},
			}}
			router := newResponseRecoveryRouter(t, gateway, true)
			request := responsesRecoveryRequest(`{"model":"gpt-6","previous_response_id":"resp-dead","session_id":"verified-session","input":[{"role":"user"},{"role":"assistant"},{"role":"user"}]}`)
			exchange, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			if len(gateway.requests) != tc.wantAttempts {
				t.Fatalf("malformed 400 triggered replay: attempts=%d want=%d", len(gateway.requests), tc.wantAttempts)
			}
			if tc.wantAttempts == 1 && exchange.Result.Response.StatusCode != http.StatusBadRequest {
				t.Fatalf("malformed 400 was not preserved: %d", exchange.Result.Response.StatusCode)
			}
		})
	}
}
