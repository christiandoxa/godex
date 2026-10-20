package routing

import (
	"context"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// Tagged RuntimeSseTapState processes each response event in order; an
// event's new turn_state supersedes the shared state for *later* IDs only.
// Already-bound response IDs retain their original turn token. A subsequent
// first turn also backfills earlier IDs without turn-state evidence.
func TestProdex04383SSESeparateTurnStateOwnersFollowEventOrder(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, wire, first, second string
		}{
			{"different-event-turns",
				"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-first\",\"turn_state\":\"turn-first\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-second\",\"turn_state\":\"turn-second\"}}\n\n", "turn-first", "turn-second"},
			{"second-event-inherits-first",
				"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-first\",\"turn_state\":\"turn-first\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-second\"}}\n\n", "turn-first", "turn-first"},
			{"late-turn-backfills-first",
				"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-first\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-second\",\"turn_state\":\"turn-second\"}}\n\n", "turn-second", "turn-second"},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				config := Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				if err := router.Observe(t.Context(), websocketQuotaOwnerA, make(http.Header), []byte(tc.wire), true); err != nil {
					t.Fatal(err)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for _, item := range []struct{ id, turn string }{{"resp-first", tc.first}, {"resp-second", tc.second}} {
					owner, err := store.owner(t.Context(), affinityKeys{previous: item.id}, router.now())
					if err != nil || owner != websocketQuotaOwnerA {
						t.Fatalf("response %s owner=%q err=%v", item.id, owner, err)
					}
					if got := router.affinity.responseTurnState(item.id, websocketQuotaOwnerA, router.now()); got != item.turn {
						t.Fatalf("response %s turn_state=%q want=%q", item.id, got, item.turn)
					}
				}
				for _, turn := range []string{"turn-first", "turn-second"} {
					want := ""
					if turn == tc.first || turn == tc.second {
						want = websocketQuotaOwnerA
					}
					owner, err := store.owner(t.Context(), affinityKeys{turn: turn}, router.now())
					if err != nil || owner != want {
						t.Fatalf("turn %s owner=%q want=%q err=%v", turn, owner, want, err)
					}
				}
			})
		}
	}
}
