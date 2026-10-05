package routing

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestRouteCircuitPersistsSoftensAndReservesOneProbe(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0)
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true},
	}
	router, err := NewRouter(Config{
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
		Now:      func() time.Time { return now }, RoutingState: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	for range 4 {
		router.recordRouteFailure(ctx, "account-a", selection)
	}
	if got := router.routeCircuitRemaining("account-a", selection, now); got != 20*time.Second {
		t.Fatalf("open circuit remaining = %s, want 20s", got)
	}
	ordered := router.orderCandidates(accounts, selection, now)
	if ordered[0].ID != "account-b" {
		t.Fatalf("candidate during open circuit = %q, want account-b", ordered[0].ID)
	}

	now = now.Add(time.Second)
	restarted, err := NewRouter(Config{
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
		Now:      func() time.Time { return now }, RoutingState: routingrepo.NewStore(root),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.routeCircuitRemaining("account-a", selection, now); got != 5*time.Second {
		t.Fatalf("restart-softened circuit = %s, want 5s", got)
	}

	now = now.Add(5 * time.Second)
	var allowed atomic.Int32
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			ok, err := restarted.reserveRouteCircuitProbe(ctx, "account-a", selection, now)
			if err != nil {
				t.Error(err)
				return
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	group.Wait()
	if allowed.Load() != 1 {
		t.Fatalf("half-open probes allowed = %d, want 1", allowed.Load())
	}

	restarted.recordRouteSuccess(ctx, "account-a", selection)
	circuits, err := store.LoadRouteCircuits(ctx, now, nil)
	if err != nil || len(circuits) != 0 {
		t.Fatalf("route circuits after success = %+v, error = %v", circuits, err)
	}
}
