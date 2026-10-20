package routing

import (
	"context"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
	"net/http"
	"testing"
)

// Tagged RuntimeSseTapState remembers the most recent turn_state across
// events delivered in separate upstream body reads. The first event can
// be part of the precommit prefix and must remain in the same lineage.
func TestProdex04383SSECrossChunkTurnLineage(t *testing.T) {
	first := []byte("data: " + `{"type":"response.output_text.delta","response":{"id":"resp-prefix","turn_state":"turn-prefix"},"delta":"first"}` + "\n\n")
	second := []byte(`{"type":"response.completed","response":{"id":"resp-followup"}}`)
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
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
			if err := router.Observe(t.Context(), websocketQuotaOwnerA, http.Header{}, first, true); err != nil {
				t.Fatal(err)
			}
			tracker := router.NewSSELineageTracker04383(websocketQuotaOwnerA, "openai", http.Header{})
			tracker.Seed([][]byte{[]byte(`{"type":"response.output_text.delta","response":{"id":"resp-prefix","turn_state":"turn-prefix"},"delta":"first"}`)})
			if err := tracker.Observe(t.Context(), second); err != nil {
				t.Fatal(err)
			}
			if turn := router.affinity.responseTurnState("resp-followup", websocketQuotaOwnerA, router.now()); turn != "turn-prefix" {
				t.Fatalf("second chunk lost active stream turn state: %q", turn)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			for _, id := range []string{"resp-prefix", "resp-followup"} {
				owner, err := store.owner(t.Context(), affinityKeys{previous: id}, router.now())
				if err != nil || owner != websocketQuotaOwnerA {
					t.Fatalf("response %s owner=%q err=%v", id, owner, err)
				}
			}
		})
	}
}
