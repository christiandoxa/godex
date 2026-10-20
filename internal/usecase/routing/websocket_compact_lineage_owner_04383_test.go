package routing

import (
	"context"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// This fixture is paired with the tagged 0.438.3 two-executable Compact
// lineage oracle. The transport quota route is WebSocket, NOT Compact.
func TestProdex04383WebSocketCompactTurnLineageOverridesActiveOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, bound := range []bool{false, true} {
			name := "memory"
			if durable {
				name = "durable"
			}
			if bound {
				name += "/verified-lineage"
			} else {
				name += "/unbound-control"
			}
			t.Run(name, func(t *testing.T) {
				gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
					websocketCommittedTestResponse("resp-compact-lineage"),
				}}
				config := Config{
					Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
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
				// Keep an unrelated original hard owner: no implementation may
				// achieve this test by clearing/rotating all existing turns.
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
					affinityKeys{turn: "original-turn"}, router.now()); err != nil {
					t.Fatal(err)
				}
				if bound {
					if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
						affinityKeys{turn: "__compact_turn_state__:compact-turn"}, router.now()); err != nil {
						t.Fatal(err)
					}
				}
				body := `{"type":"response.create","model":"gpt-6","x-codex-turn-state":"compact-turn","input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
				request := websocketDispatchRequest(body, 843325)
				request.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				want := websocketQuotaOwnerA
				if bound {
					want = websocketQuotaOwnerB
				}
				if result.Result.AccountID != want || len(gateway.accounts) != 1 || gateway.accounts[0] != want {
					t.Fatalf("compact lineage selected wrong WebSocket owner: want=%q result=%q attempts=%v", want, result.Result.AccountID, gateway.accounts)
				}
				if got := gateway.requests[0].Header.Get("x-codex-turn-state"); got != "compact-turn" {
					t.Fatalf("upstream turn-state header=%q, want compact-turn", got)
				}
				if got := string(gateway.requests[0].Body); got != body || !strings.Contains(got, `"content":"followup"`) {
					t.Fatalf("Compact lineage request corrupted opaque history: %s", got)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				lineage, err := store.owner(t.Context(), affinityKeys{turn: "__compact_turn_state__:compact-turn"}, router.now())
				if err != nil {
					t.Fatal(err)
				}
				wantLineage := ""
				if bound {
					wantLineage = websocketQuotaOwnerB
				}
				if lineage != wantLineage {
					t.Fatalf("Compact lineage durable owner mutated: got=%q want=%q", lineage, wantLineage)
				}
				original, err := store.owner(t.Context(), affinityKeys{turn: "original-turn"}, router.now())
				if err != nil || original != websocketQuotaOwnerA {
					t.Fatalf("unrelated hard owner changed: %q, %v", original, err)
				}
			})
		}
	}
}
