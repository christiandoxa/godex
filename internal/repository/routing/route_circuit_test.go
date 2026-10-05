package routing

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRouteCircuitsPersistSoftenAndClear(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_000_000, 0)
	store := NewStore(t.TempDir())
	circuit, opened, err := store.OpenRouteCircuit(ctx, "account-a", "responses", 4, now)
	if err != nil || !opened {
		t.Fatal(err)
	}
	loaded, err := NewStore(store.root).LoadRouteCircuits(ctx, now, nil)
	if err != nil || len(loaded) != 1 || loaded[0].AccountID != circuit.AccountID ||
		loaded[0].Route != circuit.Route || loaded[0].UntilUnix != now.Add(5*time.Second).Unix() {
		t.Fatalf("loaded route circuits = %+v, error = %v", loaded, err)
	}
	if err := store.ClearRouteCircuit(ctx, "account-a", "responses"); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadRouteCircuits(ctx, now, nil)
	if err != nil || len(loaded) != 0 {
		t.Fatalf("cleared route circuits = %+v, error = %v", loaded, err)
	}
}

func TestRouteCircuitStoreRejectsUnsafeSnapshotsAndLowHealth(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	for _, content := range []string{`{"version":2,"circuits":[]}`, `{"version":1,"circuits":[]} {}`} {
		root := t.TempDir()
		path := filepath.Join(root, "route-circuits.json")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewStore(root).LoadRouteCircuits(context.Background(), now, nil); err == nil {
			t.Fatalf("invalid route-circuit snapshot accepted: %s", content)
		}
	}
	if _, opened, err := NewStore(t.TempDir()).OpenRouteCircuit(context.Background(), "account-a", "responses", 3, now); err != nil || opened {
		t.Fatalf("low-health route circuit = opened %t, error %v", opened, err)
	}
}

func TestRouteCircuitProbeReservationIsAtomicAcrossStores(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	openedAt := time.Unix(100, 0)
	store := NewStore(root)
	circuit, opened, err := store.OpenRouteCircuit(ctx, "account-a", "responses", 4, openedAt)
	if err != nil || !opened {
		t.Fatal(err)
	}
	probeAt := time.Unix(circuit.UntilUnix, 0)
	var allowed atomic.Int32
	var group sync.WaitGroup
	errors := make(chan error, 12)
	for range cap(errors) {
		group.Add(1)
		go func() {
			defer group.Done()
			_, ok, err := NewStore(root).ReserveRouteCircuitProbe(ctx, "account-a", "responses", 4, probeAt)
			if err != nil {
				errors <- err
				return
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if allowed.Load() != 1 {
		t.Fatalf("independent stores reserved %d half-open probes, want 1", allowed.Load())
	}
}

func TestConcurrentRouteCircuitFailuresRetainReopenStage(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0)
	store := NewStore(t.TempDir())
	first, opened, err := store.OpenRouteCircuit(ctx, "account-a", "responses", 4, now)
	if err != nil || !opened {
		t.Fatalf("initial circuit = %+v, opened = %t, error = %v", first, opened, err)
	}
	probeAt := time.Unix(first.UntilUnix, 0)
	if _, allowed, err := store.ReserveRouteCircuitProbe(ctx, "account-a", "responses", 4, probeAt); err != nil || !allowed {
		t.Fatalf("half-open probe allowed = %t, error = %v", allowed, err)
	}
	reopenedAt := time.Unix(first.UntilUnix+5, 0)
	second, opened, err := NewStore(store.root).OpenRouteCircuit(ctx, "account-a", "responses", 5, reopenedAt)
	if err != nil || !opened || second.ReopenStage != 1 || second.UntilUnix != reopenedAt.Add(80*time.Second).Unix() {
		t.Fatalf("reopened circuit = %+v, opened = %t, error = %v", second, opened, err)
	}
}
