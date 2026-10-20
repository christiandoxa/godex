package routing

import (
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// The 0.438.3 serde_json 1.0.151 wrapper has a 128-stack recursion limit.
// The parser rejects an entire input when its 128th object/array is entered,
// even if a valid usage/identity appears earlier than the deep sibling.
func TestProdex04383NativeJSONRejectsOverdeepUnrelatedFields(t *testing.T) {
	for _, tc := range []struct {
		name, nested string
		valid        bool
	}{
		{"object127", strings.Repeat(`{"a":`, 126) + `null` + strings.Repeat(`}`, 126), true},
		{"object128", strings.Repeat(`{"a":`, 127) + `null` + strings.Repeat(`}`, 127), false},
		{"array127", strings.Repeat(`[`, 126) + `0` + strings.Repeat(`]`, 126), true},
		{"array128", strings.Repeat(`[`, 127) + `0` + strings.Repeat(`]`, 127), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"type":"response.completed","usage":{"cached_input_tokens":91},"z":` + tc.nested + `}`)
			counts, found := terminalTokenUsage(payload)
			if found != tc.valid || (found && counts.CachedInputTokens != 91) {
				t.Fatalf("native JSON %s accepted invalid depth: %+v found=%t want=%t", tc.name, counts, found, tc.valid)
			}
			if found {
				return
			}
			if got := promptCacheKeyFromBody([]byte(`{"prompt_cache_key":"known-key","z":` + tc.nested + `}`)); got != "" {
				t.Fatalf("overdeep request gave prompt-cache key %q", got)
			}
			request := proxymodel.Request{Header: make(http.Header), Body: []byte(`{"session_id":"known-session","z":` + tc.nested + `}`), WebSocketMessage: true}
			if keys := requestAffinity(request, request.Body); keys.session != "" || keys.previous != "" || keys.turn != "" {
				t.Fatalf("overdeep request gave affinity %+v", keys)
			}
			if meta := parseWebSocketRequestMetadata(request); meta.sessionID != "" || meta.previousResponseID != "" || meta.turnState != "" {
				t.Fatalf("overdeep websocket request has metadata %+v", meta)
			}
			response := []byte(`{"object":"response","id":"resp-overdeep","z":` + tc.nested + `}`)
			keys, ids := responseAffinityWithIDs(nil, response, false)
			if keys.previous != "" || len(ids) > 0 {
				t.Fatalf("overdeep response minted identities: %+v %v", keys, ids)
			}
		})
	}
}
