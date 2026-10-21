package routing

import (
	"context"
	"net/http"
	"os"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

// The tagged RuntimeSseTapState only recognizes error evidence after the
// full data: payload parses as serde_json::Value. Plain text can be a
// previous-response error in an HTTP 400 body, but NOT an SSE JSON event.
func TestProdex04383CommittedSSEPlainTextCannotReleaseVerifiedResponse(t *testing.T) {
	for _, durable := range []bool{false, true} {
		label := "memory"
		if durable {
			label = "durable"
		}
		t.Run(label, func(t *testing.T) {
			const account = websocketQuotaOwnerA
			home := t.TempDir()
			if err := os.Chmod(home, 0o700); err != nil {
				t.Fatal(err)
			}
			config := Config{Accounts: func(context.Context) ([]proxymodel.Account, error) {
				return []proxymodel.Account{{ID: account, Home: home, Enabled: true}}, nil
			}}
			if durable {
				config.Bindings = routingrepo.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if _, err := router.loadAccounts(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := router.affinity.remember(t.Context(), account, affinityKeys{previous: "resp-main"}, router.now()); err != nil {
				t.Fatal(err)
			}
			first := []byte(`{"type":"response.output_text.delta","response":{"id":"resp-live","turn_state":"state-live"},"delta":"ok"}`)
			if err := router.Observe(t.Context(), account, make(http.Header), []byte("data: "+string(first)+"\n\n"), true, "openai"); err != nil {
				t.Fatal(err)
			}
			tracker := router.NewSSELineageTracker04383(account, "openai", make(http.Header), "resp-main")
			tracker.Seed([][]byte{first})
			if err := tracker.Observe(t.Context(), []byte("previous_response_not_found")); err != nil {
				t.Fatal(err)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			for _, id := range []string{"resp-main", "resp-live"} {
				got, err := store.owner(t.Context(), affinityKeys{previous: id}, router.now())
				if err != nil || got != account {
					t.Fatalf("plain text incorrectly cleared %s owner=%q err=%v", id, got, err)
				}
			}
			got, err := store.owner(t.Context(), affinityKeys{turn: "state-live"}, router.now())
			if err != nil || got != account {
				t.Fatalf("plain text cleared turn binding owner=%q err=%v", got, err)
			}
		})
	}
}
