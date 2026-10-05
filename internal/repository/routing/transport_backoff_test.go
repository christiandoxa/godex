package routing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestTransportBackoffPersistsSoftensAndClearsByRoute(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	now := time.Unix(1_000_000, 0)
	store := NewStore(root)
	for _, backoff := range []routingentity.TransportBackoff{
		{AccountID: "account-a", Route: "responses", UntilUnix: now.Add(90 * time.Second).Unix()},
		{AccountID: "account-a", Route: "compact", UntilUnix: now.Add(30 * time.Second).Unix()},
	} {
		if err := store.SetTransportBackoff(ctx, backoff, now); err != nil {
			t.Fatal(err)
		}
	}
	shorter := routingentity.TransportBackoff{
		AccountID: "account-a", Route: "responses", UntilUnix: now.Add(20 * time.Second).Unix(),
	}
	if err := store.SetTransportBackoff(ctx, shorter, now); err != nil {
		t.Fatal(err)
	}

	loaded, err := NewStore(root).LoadTransportBackoffs(ctx, now)
	if err != nil || len(loaded) != 2 {
		t.Fatalf("transport backoffs after restart = %+v, error = %v", loaded, err)
	}
	for _, backoff := range loaded {
		if backoff.UntilUnix != now.Add(routingentity.InitialTransportBackoffDuration).Unix() {
			t.Fatalf("startup transport backoff = %+v, want 15-second softening", backoff)
		}
	}
	if err := store.ClearTransportBackoff(ctx, "account-a", "responses"); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadTransportBackoffs(ctx, now)
	if err != nil || len(loaded) != 1 || loaded[0].Route != "compact" {
		t.Fatalf("route-specific transport clear = %+v, error = %v", loaded, err)
	}
	loaded, err = store.LoadTransportBackoffs(ctx, now.Add(16*time.Second))
	if err != nil || len(loaded) != 0 {
		t.Fatalf("expired transport backoffs = %+v, error = %v", loaded, err)
	}
}

func TestConcurrentTransportBackoffsDoNotLoseRoutes(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_000_000, 0)
	const updates = 12
	var group sync.WaitGroup
	errors := make(chan error, updates)
	for index := range updates {
		group.Add(1)
		go func() {
			defer group.Done()
			err := NewStore(root).SetTransportBackoff(context.Background(), routingentity.TransportBackoff{
				AccountID: fmt.Sprintf("account-%d", index), Route: "responses",
				UntilUnix: now.Add(time.Minute).Unix(),
			}, now)
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
	loaded, err := NewStore(root).LoadTransportBackoffs(context.Background(), now)
	if err != nil || len(loaded) != updates {
		t.Fatalf("concurrent transport backoffs = %d, error = %v", len(loaded), err)
	}
}

func TestTransportBackoffSnapshotRejectsInvalidState(t *testing.T) {
	for _, content := range []string{
		`{"version":2,"backoffs":[]}`,
		`{"version":1,"backoffs":[]} {}`,
		`{"version":1,"backoffs":[{"account_id":"account-a","route":"responses","until_unix":1},{"account_id":"account-a","route":"responses","until_unix":2}]}`,
	} {
		root := t.TempDir()
		path := filepath.Join(root, "transport-backoff.json")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewStore(root).LoadTransportBackoffs(context.Background(), time.Unix(1_000_000, 0)); err == nil {
			t.Fatalf("invalid transport backoff snapshot accepted: %s", content)
		}
	}
}
