package routing

import (
	"context"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// A session conflict marker is an unusable soft preference, not a fatal
// request-owner error. Tagged 0.438.3 selects a healthy fresh profile for a
// session-only WebSocket followup; hard turn/previous conflicts stay closed.
func TestProdex04383WebSocketSessionConflictFallsBackToFreshProfile(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				websocketCommittedTestResponse("resp-session-after-conflict"),
			}}
			config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil },
			}
			if durable {
				config.Bindings = routingrepository.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			for _, owner := range []string{websocketQuotaOwnerB, websocketQuotaOwnerA} {
				if err := router.affinity.rememberVerified(t.Context(), owner,
					affinityKeys{session: "shadow-session"}, router.now()); err != nil {
					t.Fatal(err)
				}
			}
			if owner, err := router.affinity.owner(t.Context(), affinityKeys{session: "shadow-session"}, router.now()); err == nil || owner != "" {
				t.Fatalf("synthetic conflict marker not established: owner=%q err=%v", owner, err)
			}
			body := `{"type":"response.create","model":"gpt-6","session_id":"shadow-session","input":[{"role":"user","content":"continue"}]}`
			request := websocketDispatchRequest(body, 843333)
			request.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
			result, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatalf("soft session conflict rejected instead of fresh routing: %v", err)
			}
			defer result.Close()
			if result.Result.AccountID != websocketQuotaOwnerA || len(gateway.accounts) != 1 ||
				gateway.accounts[0] != websocketQuotaOwnerA || string(gateway.requests[0].Body) != body {
				t.Fatalf("soft conflicted session changed selected account or opaque input: result=%q attempts=%v", result.Result.AccountID, gateway.accounts)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			if owner, err := store.owner(t.Context(), affinityKeys{session: "shadow-session"}, router.now()); err == nil || owner != "" {
				t.Fatalf("fresh success erased durable conflict marker: owner=%q err=%v", owner, err)
			}
			if owner, err := store.owner(t.Context(), affinityKeys{previous: "resp-session-after-conflict"}, router.now()); err != nil || owner != websocketQuotaOwnerA {
				t.Fatalf("new response owner not verified: owner=%q err=%v", owner, err)
			}
		})
	}
}
