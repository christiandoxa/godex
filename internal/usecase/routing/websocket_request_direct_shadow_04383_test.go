package routing

import (
	"context"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// Tagged Prodex 0.438.3's runtime_request_semantic_plan selects an existing
// direct JSON member BEFORE attempting client_metadata, even when that member
// is blank or not a string. An invalid direct value must not silently borrow
// another session/turn binding from nested metadata and cross its owner.
func TestProdex04383WebSocketDirectMetadataShadowsNestedEvenWhenInvalid(t *testing.T) {
	for _, tc := range []struct {
		name, direct string
	}{
		{"empty string", `""`},
		{"whitespace", `"  "`},
		{"null", `null`},
		{"boolean", `false`},
		{"numeric", `123`},
		{"object", `{"value":"wrong"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := proxymodel.Request{
				WebSocketMessage: true,
				Header:           make(http.Header),
				Body: []byte(`{"type":"response.create","session_id":` + tc.direct +
					`,"x-codex-turn-state":` + tc.direct +
					`,"client_metadata":{"session_id":"nested-session","x-codex-turn-state":"nested-turn"},"input":[]}`),
			}
			got := parseWebSocketRequestMetadata(request)
			if got.sessionID != "" || got.turnState != "" {
				t.Fatalf("present but invalid direct value unexpectedly borrowed a foreign nested owner: %+v", got)
			}
			request.Header.Set("Session-Id", "header-session")
			request.Header.Set("X-Codex-Turn-State", "header-turn")
			got = parseWebSocketRequestMetadata(request)
			if got.sessionID != "header-session" || got.turnState != "header-turn" {
				t.Fatalf("invalid direct value changed explicit handshake fallback: %+v", got)
			}
		})
	}
}

func TestProdex04383WebSocketAbsentDirectStillUsesNestedMetadata(t *testing.T) {
	request := proxymodel.Request{WebSocketMessage: true, Header: make(http.Header),
		Body: []byte(`{"client_metadata":{"session_id":"nested-session","x-codex-turn-state":"nested-turn"}}`),
	}
	got := parseWebSocketRequestMetadata(request)
	if got.sessionID != "nested-session" || got.turnState != "nested-turn" {
		t.Fatalf("missing direct property lost legitimate nested fallback: %+v", got)
	}
}

// Inbound WebSocket owner selection must use the same typed metadata plan as
// the tagged adapter. The generic HTTP affinity decoder must not reintroduce
// a nested binding that the WebSocket metadata parser intentionally rejected.
func TestProdex04383WebSocketInvalidDirectDoesNotSelectNestedOwner(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		name := "memory"
		if persistent {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				websocketCommittedTestResponse("resp-fresh"),
			}}
			accounts := websocketQuotaAccounts()
			config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerB,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
			}
			if persistent {
				config.Bindings = routingrepository.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
				affinityKeys{turn: "nested-turn", session: "nested-session"}, router.now()); err != nil {
				t.Fatal(err)
			}
			request := websocketDispatchRequest(`{"type":"response.create","session_id":"","x-codex-turn-state":false,"client_metadata":{"session_id":"nested-session","x-codex-turn-state":"nested-turn"},"input":[]}`, 843318)
			result, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if result.Result.AccountID != websocketQuotaOwnerB || len(gateway.accounts) != 1 || gateway.accounts[0] != websocketQuotaOwnerB {
				t.Fatalf("invalid direct metadata selected stale nested owner: routed=%q attempts=%v", result.Result.AccountID, gateway.accounts)
			}
			if policy := gateway.requests[0].WebSocketPolicy; policy.RequestTurnState || policy.RequestSession || !policy.PromoteCommittedProfile {
				t.Fatalf("invalid direct metadata became an owned followup: %+v", policy)
			}
			for key, value := range map[string]affinityKeys{
				"turn": {turn: "nested-turn"}, "session": {session: "nested-session"},
			} {
				owner, err := router.affinity.owner(t.Context(), value, router.now())
				if err != nil || owner != websocketQuotaOwnerA {
					t.Fatalf("%s owner changed by shadowed metadata: owner=%q err=%v", key, owner, err)
				}
			}
		})
	}
}
