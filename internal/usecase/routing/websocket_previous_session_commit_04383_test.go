package routing

import (
	"context"
	"testing"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// Tagged Prodex 0.438.3 commits direct session ownership after a successful
// WebSocket response even when an earlier response ID hard-pinned the request
// and a different profile still owns its soft Compact session alias.
func TestProdex04383WebSocketPreviousResponseCommitPersistsDirectSession(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				websocketCommittedTestResponse("resp-new-session"),
			}}
			config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerB,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return websocketQuotaAccounts(), nil
				},
			}
			if durable {
				config.Bindings = routingrepository.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
				affinityKeys{previous: "resp-main"}, router.now()); err != nil {
				t.Fatal(err)
			}
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
				affinityKeys{session: "__compact_session__:session-alias"}, router.now()); err != nil {
				t.Fatal(err)
			}
			body := `{"type":"response.create","model":"gpt-6","previous_response_id":"resp-main","session_id":"session-alias","input":[{"role":"user","content":"full context"}]}`
			request := websocketDispatchRequest(body, 843331)
			request.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
			response, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Close()
			if response.Result.AccountID != websocketQuotaOwnerA || len(gateway.accounts) != 1 ||
				gateway.accounts[0] != websocketQuotaOwnerA || string(gateway.requests[0].Body) != body {
				t.Fatalf("hard previous-response owner lost: account=%q attempts=%v", response.Result.AccountID, gateway.accounts)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			for _, tc := range []struct {
				name string
				keys affinityKeys
				want string
			}{
				{"direct session", affinityKeys{session: "session-alias"}, websocketQuotaOwnerA},
				{"Compact alias", affinityKeys{session: "__compact_session__:session-alias"}, websocketQuotaOwnerB},
				{"new response", affinityKeys{previous: "resp-new-session"}, websocketQuotaOwnerA},
			} {
				owner, err := store.owner(t.Context(), tc.keys, router.now())
				if err != nil || owner != tc.want {
					t.Fatalf("%s owner=%q want=%q err=%v", tc.name, owner, tc.want, err)
				}
			}
		})
	}
}

// On a successful previous-response-owned WebSocket turn, Prodex records a
// verified direct session observation. If that raw session was previously
// owned by another account, it becomes a hard conflict marker; the existing
// Compact alias is never taken over or silently treated as a valid owner.
func TestProdex04383WebSocketPreviousResponseCommitsConflictingSessionMarker(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				websocketCommittedTestResponse("resp-conflicted-session"),
			}}
			config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerB,
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
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
				affinityKeys{previous: "resp-main"}, router.now()); err != nil {
				t.Fatal(err)
			}
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
				affinityKeys{session: "session-alias"}, router.now()); err != nil {
				t.Fatal(err)
			}
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
				affinityKeys{session: "__compact_session__:session-alias"}, router.now()); err != nil {
				t.Fatal(err)
			}
			body := `{"type":"response.create","model":"gpt-6","previous_response_id":"resp-main","session_id":"session-alias","input":[{"role":"user","content":"full context"}]}`
			request := websocketDispatchRequest(body, 843332)
			request.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
			response, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Close()
			if response.Result.AccountID != websocketQuotaOwnerA || len(gateway.accounts) != 1 || gateway.accounts[0] != websocketQuotaOwnerA {
				t.Fatalf("hard previous-response owner changed: result=%q attempts=%v", response.Result.AccountID, gateway.accounts)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			if owner, err := store.owner(t.Context(), affinityKeys{session: "session-alias"}, router.now()); err == nil || owner != "" {
				t.Fatalf("verified conflicting direct session not rejected: owner=%q err=%v", owner, err)
			}
			key := affinityDigest("session", "session-alias")
			actualOwner := ""
			if durable {
				bindings, err := config.Bindings.Load(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				for _, binding := range bindings {
					if binding.Key == key {
						actualOwner = binding.AccountID
					}
				}
			} else {
				router.affinity.mu.Lock()
				actualOwner = router.affinity.values[key].accountID
				router.affinity.mu.Unlock()
			}
			if actualOwner != routingentity.ConflictAccountID {
				t.Fatalf("durable direct session marker = %q, want hard conflict", actualOwner)
			}
			for _, tc := range []struct {
				keys affinityKeys
				want string
			}{
				{affinityKeys{previous: "resp-main"}, websocketQuotaOwnerA},
				{affinityKeys{previous: "resp-conflicted-session"}, websocketQuotaOwnerA},
				{affinityKeys{session: "__compact_session__:session-alias"}, websocketQuotaOwnerB},
			} {
				owner, err := store.owner(t.Context(), tc.keys, router.now())
				if err != nil || owner != tc.want {
					t.Fatalf("unrelated verified binding changed: got=%q want=%q err=%v", owner, tc.want, err)
				}
			}
		})
	}
}
