package openai

import (
	"bytes"
	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	"testing"
)

// Prodex 0.438.0 crates/prodex-runtime-proxy/src/websocket_message.rs
// terminal_frame_plan and tagged Mojo ABI treat the *presence* of `status`
// as authoritative; an invalid `status` cannot fall back to `status_code`.
// The upper numeric bound is unsigned u16, not an HTTP-only 999 maximum.
func TestProdex04380WebSocketTerminalFrameStatusPriorityAndBounds(t *testing.T) {
	cases := []struct {
		name, payload string
		terminal      bool
		status        int
	}{
		{"status overrides status code", `{"type":"error","status":399,"status_code":400}`, false, 399},
		{"non numeric status blocks secondary", `{"type":"error","status":"invalid","status_code":400}`, false, 0},
		{"null status blocks secondary", `{"type":"error","status":null,"status_code":400}`, false, 0},
		{"fractional status blocks secondary", `{"type":"error","status":399.5,"status_code":400}`, false, 0},
		{"negative status blocks secondary", `{"type":"error","status":-1,"status_code":400}`, false, 0},
		{"out of range status blocks secondary", `{"type":"error","status":65536,"status_code":400}`, false, 0},
		{"valid high u16 status", `{"type":"error","status":1000}`, true, 1000},
		{"max u16 status", `{"type":"error","status":65535}`, true, 65535},
		{"larger than u16", `{"type":"error","status":65536}`, false, 0},
		{"float status not accepted", `{"type":"error","status":400.1}`, false, 0},
		{"normal secondary field", `{"type":"error","status_code":400}`, true, 400},
		{"malformed secondary field", `{"type":"error","status_code":"400"}`, false, 0},
		{"nonerror frame not terminal", `{"type":"response.output_text.delta","status":503}`, false, 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(tc.payload)
			if got := websocketPayloadStatus(payload); got != tc.status {
				t.Errorf("status priority/bounds for %s = %d, want %d", tc.payload, got, tc.status)
			}
			kind, _, _ := websocketEventMetadata(payload)
			if got := websocketPayloadTerminal(kind, payload); got != tc.terminal {
				t.Errorf("terminal for %s = %t, want %t", tc.payload, got, tc.terminal)
			}
		})
	}
}

func TestProdex04380WebSocketTerminalFrameConnectionLimitFromEitherCodeField(t *testing.T) {
	cases := []struct {
		name, payload string
		want          bool
	}{
		{"nested uppercase", `{"type":"error","error":{"code":"WEBSOCKET_CONNECTION_LIMIT_REACHED"}}`, true},
		{"top-level", `{"type":"error","code":"websocket_connection_limit_reached"}`, true},
		{"top-level even if nested has unrelated code", `{"type":"error","error":{"code":"other"},"code":"websocket_connection_limit_reached"}`, true},
		{"top-level with non-object nested error", `{"type":"error","error":"other","code":"websocket_connection_limit_reached"}`, true},
		{"top-level with numeric nested error", `{"type":"error","error":{"code":400},"code":"websocket_connection_limit_reached"}`, true},
		{"top-level with null nested error", `{"type":"error","error":null,"code":"websocket_connection_limit_reached"}`, true},

		{"nested even if top-level has unrelated code", `{"type":"error","error":{"code":"websocket_connection_limit_reached"},"code":"other"}`, true},
		{"code has leading whitespace", `{"type":"error","code":" websocket_connection_limit_reached"}`, false},
		{"code has trailing whitespace", `{"type":"error","error":{"code":"websocket_connection_limit_reached "}}`, false},
		{"Unicode lookalike s cannot fold", `{"type":"error","code":"webſocket_connection_limit_reached"}`, false},

		{"ordinary top-level rate limit", `{"type":"error","code":"rate_limit_exceeded"}`, false},
		{"nonterminal progress with error-like code", `{"type":"response.output_text.delta","code":"websocket_connection_limit_reached"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(tc.payload)
			if got := isWebSocketConnectionLimit(payload); got != tc.want {
				t.Errorf("limit code %q = %t, want %t", tc.payload, got, tc.want)
			}
			kind, _, _ := websocketEventMetadata(payload)
			terminal := websocketPayloadTerminal(kind, payload)
			if kind == "error" && terminal != tc.want {
				t.Errorf("terminal connection limit %q = %t, want %t", tc.payload, terminal, tc.want)
			}
			if kind != "error" && terminal {
				t.Errorf("progress event must not commit solely from code text: %q", tc.payload)
			}
		})
	}
}

// The readWebSocketEvent path, not just isolated helper functions, must make
// the same terminal and precommit classification. Wire bytes are forwarded
// unmodified while the control decision uses typed JSON facts.
type inMemoryServerFrames04380 struct{ bytes.Buffer }

func (*inMemoryServerFrames04380) Close() error { return nil }

func TestProdex04380WebSocketDecodedFrameRetainsTaggedTerminality(t *testing.T) {
	cases := []struct {
		name, body string
		terminal   bool
		retry      string
	}{
		{"invalid status blocks secondary 400", `{"type":"error","status":"invalid","status_code":400}`, false, ""},
		{"unparsable u16 blocks secondary 400", `{"type":"error","status":65536,"status_code":400}`, false, ""},
		{"valid 65535 terminal", `{"type":"error","status":65535}`, true, ""},
		{"nested 429 status terminal", `{"type":"error","status_code":429}`, true, ""},
		{"connection limit top overrides nested other", `{"type":"error","error":{"code":"unknown_error"},"code":"WEBSOCKET_CONNECTION_LIMIT_REACHED"}`, true, "websocket_connection_limit_reached"},
		{"malformed nested error does not mask top", `{"type":"error","error":"opaque","code":"websocket_connection_limit_reached"}`, true, "websocket_connection_limit_reached"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := &inMemoryServerFrames04380{}
			if err := websocketframe.WriteFrame(input, 1, []byte(tc.body), false); err != nil {
				t.Fatal(err)
			}
			original := append([]byte(nil), input.Bytes()...)
			got, err := readWebSocketEvent(input)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.frames, original) || !bytes.Equal(got.payload, []byte(tc.body)) {
				t.Fatal("native WebSocket event bytes mutated during inspection")
			}
			if !got.text || got.kind != "error" || got.terminal != tc.terminal || got.retryCode != tc.retry {
				t.Fatalf("wire=%q text=%t kind=%q terminal=%t retry=%q, want %t/%q", tc.body, got.text, got.kind, got.terminal, got.retryCode, tc.terminal, tc.retry)
			}
		})
	}
}

// A malformed primary status is not permission to treat a secondary 503 as a
// transient overload: that would rotate accounts or replay precommit turns
// that Prodex 0.438.0 deliberately does not reclassify.
func TestProdex04380WebSocketStatusPrecedenceCannotInventTransportRetry(t *testing.T) {
	for _, tc := range []struct{ name, payload, want string }{
		{"missing primary 503 overload", `{"type":"error","status_code":503}`, "server_is_overloaded"},
		{"valid primary 503 overload", `{"type":"error","status":503,"status_code":400}`, "server_is_overloaded"},
		{"primary 400 overrides fallback 503", `{"type":"error","status":400,"status_code":503}`, ""},
		{"invalid primary blocks fallback 503", `{"type":"error","status":"invalid","status_code":503}`, ""},
		{"u16 overflow blocks fallback 503", `{"type":"error","status":65536,"status_code":503}`, ""},
		{"decimal primary blocks fallback 503", `{"type":"error","status":503.1,"status_code":503}`, ""},
		{"response text mention never triggers retry", `{"type":"response.output_text.delta","status":503,"delta":"rate_limit_exceeded"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := websocketRetryFailureCode([]byte(tc.payload)); code != tc.want {
				t.Fatalf("WebSocket payload %s retry=%q want=%q", tc.payload, code, tc.want)
			}
		})
	}
}
