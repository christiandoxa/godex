package routing

import (
	"context"
	"encoding/json"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
	"net/http"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged 0.438.3 runtime_proxy_input_item_is_tool_output checks whether the
// decoded, UNTRIMMED 'type' string ends with _call_output. Whitespace in a
// type suffix changes whether the request requires previous-response affinity.
// The call_id is separately checked for nonblank text.
func TestProdex04383WebSocketToolOutputKindSuffixPreservesSpaces(t *testing.T) {
	for _, tc := range []struct {
		name, kind, callID string
		requires           bool
	}{
		{"canonical", "function_call_output", "call-1", true},
		{"custom", "custom_tool_call_output", "call-1", true},
		{"leading-space", " function_call_output", "call-1", true},
		{"trailing-space", "function_call_output ", "call-1", false},
		{"trailing-tab", "function_call_output\t", "call-1", false},
		{"both-sides", "  function_call_output  ", "call-1", false},
		{"call-id-blank", "function_call_output", " \t ", false},
		{"call-id-leading", "function_call_output", " call-1", true},
		{"call-id-trailing", "function_call_output", "call-1 ", true},
		{"not-output", "function_call", "call-1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"type":"response.create","previous_response_id":"resp-known","input":[{"type":` + jsonQuoted04383(tc.kind) + `,"call_id":` + jsonQuoted04383(tc.callID) + `,"output":"ok"}]}`
			request := proxymodel.Request{Method: http.MethodGet, Path: "/backend-api/codex/responses",
				Body: []byte(body), WebSocketMessage: true, Header: make(http.Header)}
			metadata := parseWebSocketRequestMetadata(request)
			if metadata.requiresPreviousResponseAffinity != tc.requires {
				t.Fatalf("type=%q call_id=%q requires locked owner=%v want=%v", tc.kind, tc.callID, metadata.requiresPreviousResponseAffinity, tc.requires)
			}
		})
	}
}

func jsonQuoted04383(s string) string { v, _ := json.Marshal(s); return string(v) }

// A false locked-affinity flag consumes all three 0.438.3 owner retries
// after previous_response_not_found instead of returning a stale signal
// immediately. Preserve that observable retry budget, not just parsing.
func TestProdex04383WebSocketToolTypeTrailingSpaceDoesNotSpendLockedOwnerRetries(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, kind   string
			wantAttempts int
		}{
			{"canonical", "function_call_output", 4},
			{"trailing-space", "function_call_output ", 1},
			{"trailing-tab", "function_call_output\t", 1},
			{"leading-space", " function_call_output", 4},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
					websocketPreviousResponseFailure(""), websocketPreviousResponseFailure(""),
					websocketPreviousResponseFailure(""), websocketPreviousResponseFailure(""),
				}}
				config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
					Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				router.wait = func(context.Context, time.Duration) error { return nil }
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
					affinityKeys{previous: "resp-known"}, router.now()); err != nil {
					t.Fatal(err)
				}
				body := `{"type":"response.create","previous_response_id":"resp-known","input":[{"type":` +
					jsonQuoted04383(tc.kind) + `,"call_id":"call-1","output":"hello"}]}`
				result, err := router.Forward(t.Context(), websocketDispatchRequest(body, 843390))
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if len(gateway.accounts) != tc.wantAttempts {
					t.Fatalf("unexpected previous-recovery retry count for %q: got=%d want=%d", tc.kind, len(gateway.accounts), tc.wantAttempts)
				}
				for _, owner := range gateway.accounts {
					if owner != websocketQuotaOwnerA {
						t.Fatalf("owner retry escaped hard previous owner: %v", gateway.accounts)
					}
				}
				for _, request := range gateway.requests {
					if string(request.Body) != body {
						t.Fatal("opaque WebSocket message rewritten")
					}
				}
			})
		}
	}
}
