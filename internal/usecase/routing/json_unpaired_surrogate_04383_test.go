package routing

import (
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Prodex 0.438.3's serde_json rejects lone surrogate escape sequences even
// though Go encoding/json silently replaces them with U+FFFD. A malformed
// auxiliary string must not authorize the request's otherwise valid session.
func TestProdex04383UnpairedUnicodeEscapesCannotCreateJSONAffinity(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		valid      bool
	}{
		{"lone-high", `"\ud800"`, false},
		{"lone-low", `"\udc00"`, false},
		{"wrong-low", `"\ud800\u0041"`, false},
		{"two-high", `"\ud800\ud801"`, false},
		{"one-low-followed-by-high", `"\udc00\ud800"`, false},
		{"valid-surrogate-pair", `"\ud83d\ude00"`, true},
		{"escaped-backslash", `"\\ud800"`, true},
		{"ordinary-ascii", `"normal"`, true},
		{"escaped-quote", `"one\\\"two"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"session_id":"known-session","note":` + tc.note + `,"input":[]}`)
			request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header), Body: payload}
			want := ""
			if tc.valid {
				want = "known-session"
			}
			if got := requestAffinity(request, payload).session; got != want {
				t.Fatalf("session for %s=%q want=%q", tc.name, got, want)
			}
			request.WebSocketMessage = true
			if got := parseWebSocketRequestMetadata(request).sessionID; got != want {
				t.Fatalf("websocket session for %s=%q want=%q", tc.name, got, want)
			}
		})
	}
}

func TestProdex04383UnpairedUnicodeEscapesDoNotCreateUsageOrPromptOwner(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		valid      bool
	}{
		{"lone-in-value", `"\ud800"`, false},
		{"lone-in-key", `"key\udc00"`, false},
		{"valid-pair", `"\ud83d\ude00"`, true},
		{"escaped-slash", `"\\ud800"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var payload []byte
			if tc.name == "lone-in-key" {
				payload = []byte(`{"prompt_cache_key":"known-key",` + tc.note + `:0,"type":"response.completed","usage":{"cached_input_tokens":90}}`)
			} else {
				payload = []byte(`{"prompt_cache_key":"known-key","note":` + tc.note + `,"type":"response.completed","usage":{"cached_input_tokens":90}}`)
			}
			key := promptCacheKeyFromBody(payload)
			counts, seen := terminalTokenUsage(payload)
			if tc.valid {
				if key != "known-key" || !seen || counts.CachedInputTokens != 90 {
					t.Fatalf("valid unicode rejected: key=%q counts=%+v seen=%v", key, counts, seen)
				}
			} else if key != "" || seen || counts.CachedInputTokens != 0 {
				t.Fatalf("invalid surrogate created owner or usage: key=%q counts=%+v seen=%v", key, counts, seen)
			}
		})
	}
}
