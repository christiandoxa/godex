package routing

import (
	"context"
	"os"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// The tagged 0.438.3 release planner removes a dead turn only when no
// surviving response still refers to it. That must hold for the in-memory
// cache, a durable source, and a cold-start store whose cache has no
// knowledge of an unrelated still-live response/turn sidecar.
func TestProdex04383DeadSSETurnRemainsOwnedWithSurvivingResponse(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, survivor := range []bool{false, true} {
			for _, cold := range []bool{false, true} {
				if cold && !durable {
					continue
				}
				name := "volatile"
				if durable {
					name = "durable"
				}
				if survivor {
					name += "/surviving"
				} else {
					name += "/orphan"
				}
				if cold {
					name += "/cold"
				} else {
					name += "/warm"
				}
				t.Run(name, func(t *testing.T) {
					home := t.TempDir()
					if err := os.Chmod(home, 0o700); err != nil {
						t.Fatal(err)
					}
					owner := websocketQuotaOwnerA
					config := Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
					if durable {
						config.Bindings = routingrepository.NewStore(t.TempDir())
					}
					router, err := NewRouter(config)
					if err != nil {
						t.Fatal(err)
					}
					defer router.Close()
					now := router.now()
					for _, id := range []string{"resp-dead", "resp-live"} {
						if id == "resp-live" && !survivor {
							continue
						}
						if err := router.affinity.remember(t.Context(), owner, affinityKeys{previous: id, turn: "shared-turn"}, now); err != nil {
							t.Fatal(err)
						}
						router.rememberAccountTurnState(t.Context(), id, owner, home, "shared-turn", now)
					}
					store := router.affinity
					if cold {
						store = newAffinityStore()
						store.repository = config.Bindings
					}
					if err := store.forgetDeadResponse(t.Context(), "resp-dead", owner, home); err != nil {
						t.Fatal(err)
					}
					if err := store.releaseDeadStreamOrphanTurn04383(t.Context(), owner, home, "shared-turn", now); err != nil {
						t.Fatal(err)
					}
					want := ""
					if survivor {
						want = owner
					}
					got, err := store.owner(t.Context(), affinityKeys{turn: "shared-turn"}, now)
					if err != nil || got != want {
						t.Fatalf("turn after dead chain got=%q want=%q err=%v", got, want, err)
					}
					if survivor {
						got, err = store.owner(t.Context(), affinityKeys{previous: "resp-live"}, now)
						if err != nil || got != owner {
							t.Fatalf("surviving response lost: owner=%q err=%v", got, err)
						}
					}
				})
			}
		}
	}
}
