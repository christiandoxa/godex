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

func TestRetryBackoffPersistsExpiresAndClears(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewStore(root)
	now := time.Unix(1_000_000, 0)
	backoff := routingentity.RetryBackoff{AccountID: "account-a", UntilUnix: now.Add(20 * time.Second).Unix()}
	if err := store.SetRetryBackoff(ctx, backoff, now); err != nil {
		t.Fatal(err)
	}
	shorter := backoff
	shorter.UntilUnix = now.Add(10 * time.Second).Unix()
	if err := store.SetRetryBackoff(ctx, shorter, now); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStore(root).LoadRetryBackoffs(ctx, now.Add(9*time.Second))
	if err != nil || len(loaded) != 1 || loaded[0].UntilUnix != shorter.UntilUnix {
		t.Fatalf("latest persisted retry backoff = %+v, error = %v", loaded, err)
	}
	if err := store.ClearRetryBackoff(ctx, "account-a", now); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadRetryBackoffs(ctx, now)
	if err != nil || len(loaded) != 0 {
		t.Fatalf("cleared retry backoffs = %+v, error = %v", loaded, err)
	}
	if err := store.SetRetryBackoff(ctx, routingentity.RetryBackoff{
		AccountID: "account-b", UntilUnix: now.Add(20 * time.Second).Unix(),
	}, now); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadRetryBackoffs(ctx, now.Add(20*time.Second))
	if err != nil || len(loaded) != 0 {
		t.Fatalf("expired retry backoffs = %+v, error = %v", loaded, err)
	}
}

func TestConcurrentRetryBackoffsDoNotLoseAccounts(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_000_000, 0)
	const updates = 10
	var group sync.WaitGroup
	errors := make(chan error, updates)
	for index := range updates {
		group.Add(1)
		go func() {
			defer group.Done()
			err := NewStore(root).SetRetryBackoff(context.Background(), routingentity.RetryBackoff{
				AccountID: fmt.Sprintf("account-%d", index), UntilUnix: now.Add(time.Minute).Unix(),
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
	loaded, err := NewStore(root).LoadRetryBackoffs(context.Background(), now)
	if err != nil || len(loaded) != updates {
		t.Fatalf("concurrent retry backoffs = %d, error = %v", len(loaded), err)
	}
}

func TestRetryBackoffSnapshotRejectsInvalidState(t *testing.T) {
	for _, content := range []string{
		`{"version":2,"backoffs":[]}`,
		`{"version":1,"backoffs":[]} {}`,
		`{"version":1,"backoffs":[{"account_id":"account-a","until_unix":999999999999999999}]}`,
	} {
		root := t.TempDir()
		path := filepath.Join(root, "retry-backoff.json")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewStore(root).LoadRetryBackoffs(context.Background(), time.Unix(1_000_000, 0)); err == nil {
			t.Fatalf("invalid retry backoff snapshot accepted: %s", content)
		}
	}
}

func TestRetryBackoffLatestMutationMayShortenDeadline(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewStore(root)
	now := time.Unix(2_000_000, 0)
	if err := store.SetRetryBackoff(ctx, routingentity.RetryBackoff{
		AccountID: "account-a", UntilUnix: now.Add(60 * time.Second).Unix(),
	}, now); err != nil {
		t.Fatal(err)
	}
	later := now.Add(5 * time.Second)
	shorter := routingentity.RetryBackoff{
		AccountID: "account-a", UntilUnix: later.Add(20 * time.Second).Unix(),
	}
	if err := store.SetRetryBackoff(ctx, shorter, later); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStore(root).LoadRetryBackoffs(ctx, later)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].UntilUnix != shorter.UntilUnix {
		t.Fatalf("latest retry backoff = %+v, want until=%d", loaded, shorter.UntilUnix)
	}
}

func TestRetryBackoffClearTombstoneBlocksStaleSet(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	base := time.Unix(3_000_000, 0)
	if err := store.SetRetryBackoff(ctx, routingentity.RetryBackoff{
		AccountID: "account-a", UntilUnix: base.Add(time.Minute).Unix(),
	}, base); err != nil {
		t.Fatal(err)
	}
	clearedAt := base.Add(2 * time.Second)
	if err := store.ClearRetryBackoff(ctx, "account-a", clearedAt); err != nil {
		t.Fatal(err)
	}
	staleAt := base.Add(time.Second)
	if err := NewStore(store.root).SetRetryBackoff(ctx, routingentity.RetryBackoff{
		AccountID: "account-a", UntilUnix: staleAt.Add(time.Minute).Unix(),
	}, staleAt); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadRetryBackoffs(ctx, clearedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("stale set resurrected cleared retry backoff: %+v", loaded)
	}
}

func TestRetryBackoffStaleClearDoesNotRemoveNewerSet(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	base := time.Unix(4_000_000, 0)
	clearAt := base.Add(time.Second)
	if err := store.ClearRetryBackoff(ctx, "account-a", clearAt); err != nil {
		t.Fatal(err)
	}
	setAt := base.Add(2 * time.Second)
	backoff := routingentity.RetryBackoff{
		AccountID: "account-a", UntilUnix: setAt.Add(time.Minute).Unix(),
	}
	if err := store.SetRetryBackoff(ctx, backoff, setAt); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(store.root).ClearRetryBackoff(ctx, "account-a", clearAt); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadRetryBackoffs(ctx, setAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].UntilUnix != backoff.UntilUnix {
		t.Fatalf("stale clear removed newer retry backoff: %+v", loaded)
	}
}
