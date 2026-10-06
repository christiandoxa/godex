package routing

import (
	"context"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestRouteMemoryPersistsMutatesAndDecaysByKind(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Unix(10_000, 0)
	ctx := context.Background()
	for _, fixture := range []struct {
		kind  string
		score uint8
	}{
		{routingentity.RouteMemoryBadPairing, 3}, {routingentity.RouteMemoryPerformance, 8}, {routingentity.RouteMemorySuccessStreak, 2},
	} {
		_, err := store.MutateRouteMemory(ctx, "alpha", "responses", fixture.kind, now, func(score routingentity.RouteMemoryScore) routingentity.RouteMemoryScore {
			score.Score = fixture.score
			return score
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := NewStore(root).LoadRouteMemory(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 {
		t.Fatalf("memory count = %d", len(loaded))
	}
	aged := now.Add(301 * time.Second)
	loaded, err = NewStore(root).LoadRouteMemory(ctx, aged)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 {
		t.Fatalf("decayed memory count = %d: %#v", len(loaded), loaded)
	}
	effective := map[string]uint8{}
	for _, score := range loaded {
		effective[score.Kind] = score.Effective(aged)
	}
	if effective[routingentity.RouteMemoryBadPairing] != 2 || effective[routingentity.RouteMemoryPerformance] != 7 || effective[routingentity.RouteMemorySuccessStreak] != 1 {
		t.Fatalf("decayed memory = %#v", effective)
	}

}

func TestRouteMemoryMutationIsAtomicAcrossStores(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(20_000, 0)
	ctx := context.Background()
	bump := func(store *Store) error {
		_, err := store.MutateRouteMemory(ctx, "alpha", "responses", routingentity.RouteMemoryBadPairing, now, func(score routingentity.RouteMemoryScore) routingentity.RouteMemoryScore {
			score.Score = score.Effective(now) + 1
			return score
		})
		return err
	}
	if err := bump(NewStore(root)); err != nil {
		t.Fatal(err)
	}
	if err := bump(NewStore(root)); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStore(root).LoadRouteMemory(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Score != 2 {
		t.Fatalf("atomic memory = %#v", loaded)
	}
}
