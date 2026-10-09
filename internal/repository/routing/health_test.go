package routing

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRouteHealthPersistsAndDecays(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_000_000, 0)
	store := NewStore(root)
	for range 2 {
		if _, err := store.AdjustRouteHealth(context.Background(), "account-a", "responses", 1, now); err != nil {
			t.Fatal(err)
		}
	}

	scores, err := NewStore(root).LoadRouteHealth(context.Background(), now)
	if err != nil || len(scores) != 1 || scores[0].Score != 2 {
		t.Fatalf("reloaded route health = %+v, error = %v", scores, err)
	}
	if got := scores[0].Effective(now.Add(time.Minute)); got != 1 {
		t.Fatalf("effective score after one minute = %d, want 1", got)
	}
	scores, err = store.LoadRouteHealth(context.Background(), now.Add(2*time.Minute))
	if err != nil || len(scores) != 0 {
		t.Fatalf("decayed route health = %+v, error = %v", scores, err)
	}
}

func TestConcurrentRouteHealthUpdatesDoNotLoseScores(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_000_000, 0)
	const updates = 10
	var group sync.WaitGroup
	errors := make(chan error, updates)
	for range updates {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := NewStore(root).AdjustRouteHealth(context.Background(), "account-b", "compact", 1, now)
			errors <- err
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	scores, err := NewStore(root).LoadRouteHealth(context.Background(), now)
	if err != nil || len(scores) != 1 || scores[0].Score != updates {
		t.Fatalf("concurrent route health = %+v, error = %v", scores, err)
	}
}

func TestRouteHealthSuccessWithoutPenaltyDoesNotWriteSnapshot(t *testing.T) {
	root := t.TempDir()
	if _, err := NewStore(root).AdjustRouteHealth(
		context.Background(), "account-a", "responses", -1, time.Unix(1_000_000, 0),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "route-health.json")); !os.IsNotExist(err) {
		t.Fatalf("no-op success snapshot stat error = %v", err)
	}
}

func TestSetRouteHealthPreservesNewerOrStrongerSnapshot(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if _, err := store.SetRouteHealth(t.Context(), "account-a", "responses", 5, time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	if got, err := store.SetRouteHealth(t.Context(), "account-a", "responses", 2, time.Unix(199, 0)); err != nil {
		t.Fatal(err)
	} else if got.Score != 5 || got.UpdatedUnix != 200 {
		t.Fatalf("stale route-health update = %+v", got)
	}
	if got, err := store.SetRouteHealth(t.Context(), "account-a", "responses", 3, time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	} else if got.Score != 5 || got.UpdatedUnix != 200 {
		t.Fatalf("weaker tied route-health update = %+v", got)
	}
	if got, err := store.SetRouteHealth(t.Context(), "account-a", "responses", 6, time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	} else if got.Score != 6 || got.UpdatedUnix != 200 {
		t.Fatalf("stronger tied route-health update = %+v", got)
	}
	if got, err := store.SetRouteHealth(t.Context(), "account-a", "responses", 1, time.Unix(201, 0)); err != nil {
		t.Fatal(err)
	} else if got.Score != 1 || got.UpdatedUnix != 201 {
		t.Fatalf("newer route-health update = %+v", got)
	}
}

func TestSetRouteHealthClearsOnlyWithFreshTimestamp(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if _, err := store.SetRouteHealth(t.Context(), "account-a", "responses", 2, time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	if got, err := store.SetRouteHealth(t.Context(), "account-a", "responses", 0, time.Unix(199, 0)); err != nil {
		t.Fatal(err)
	} else if got.Score != 2 || got.UpdatedUnix != 200 {
		t.Fatalf("stale clear = %+v", got)
	}
	if _, err := store.SetRouteHealth(t.Context(), "account-a", "responses", 0, time.Unix(201, 0)); err != nil {
		t.Fatal(err)
	}
	scores, err := store.LoadRouteHealth(t.Context(), time.Unix(201, 0))
	if err != nil || len(scores) != 0 {
		t.Fatalf("fresh clear scores = %+v, error = %v", scores, err)
	}
}

func TestRouteHealthSnapshotRejectsUnsupportedAndUnsafeFiles(t *testing.T) {
	for _, content := range []string{`{"version":2,"scores":[]}`, `{"version":1,"scores":[]} {}`} {
		root := t.TempDir()
		path := filepath.Join(root, "route-health.json")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewStore(root).LoadRouteHealth(context.Background(), time.Unix(1_000_000, 0)); err == nil {
			t.Fatalf("invalid route-health snapshot accepted: %s", content)
		}
	}
}
