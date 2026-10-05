package routing

import (
	"context"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestRetryBackoffLatestUpdateMayShortenDeadline(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Unix(10_000, 0)
	account := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := store.SetRetryBackoff(t.Context(), routingentity.RetryBackoff{
		AccountID: account, UntilUnix: now.Add(60 * time.Second).Unix(),
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRetryBackoff(t.Context(), routingentity.RetryBackoff{
		AccountID: account, UntilUnix: now.Add(20 * time.Second).Unix(),
	}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadRetryBackoffs(t.Context(), now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	want := now.Add(20 * time.Second).Unix()
	if len(loaded) != 1 || loaded[0].UntilUnix != want {
		t.Fatalf("latest backoff = %+v, want until %d", loaded, want)
	}
}

func TestRetryBackoffPersistsExpiresAndClears(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Unix(20_000, 0)
	account := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	backoff := routingentity.RetryBackoff{AccountID: account, UntilUnix: now.Add(20 * time.Second).Unix()}
	if err := store.SetRetryBackoff(context.Background(), backoff, now); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStore(store.root).LoadRetryBackoffs(t.Context(), now.Add(19*time.Second))
	if err != nil || len(loaded) != 1 {
		t.Fatalf("persisted backoff = %+v err=%v", loaded, err)
	}
	if err := store.ClearRetryBackoff(t.Context(), account, now); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadRetryBackoffs(t.Context(), now)
	if err != nil || len(loaded) != 0 {
		t.Fatalf("cleared backoff = %+v err=%v", loaded, err)
	}
	if err := store.SetRetryBackoff(t.Context(), backoff, now); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadRetryBackoffs(t.Context(), now.Add(20*time.Second))
	if err != nil || len(loaded) != 0 {
		t.Fatalf("expired backoff = %+v err=%v", loaded, err)
	}
}

func TestRetryBackoffClearTombstoneBlocksStaleSet(t *testing.T) {
	store := NewStore(t.TempDir())
	account := "cccccccccccccccccccccccccccccccc"
	base := time.Unix(30_000, 0)
	if err := store.SetRetryBackoff(t.Context(), routingentity.RetryBackoff{
		AccountID: account, UntilUnix: base.Add(time.Minute).Unix(),
	}, base); err != nil {
		t.Fatal(err)
	}
	clearedAt := base.Add(2 * time.Second)
	if err := store.ClearRetryBackoff(t.Context(), account, clearedAt); err != nil {
		t.Fatal(err)
	}
	staleAt := base.Add(time.Second)
	if err := NewStore(store.root).SetRetryBackoff(t.Context(), routingentity.RetryBackoff{
		AccountID: account, UntilUnix: staleAt.Add(time.Minute).Unix(),
	}, staleAt); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadRetryBackoffs(t.Context(), clearedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("stale set resurrected cleared retry backoff: %+v", loaded)
	}
}

func TestRetryBackoffStaleClearDoesNotRemoveNewerSet(t *testing.T) {
	store := NewStore(t.TempDir())
	account := "dddddddddddddddddddddddddddddddd"
	base := time.Unix(40_000, 0)
	clearAt := base.Add(time.Second)
	if err := store.ClearRetryBackoff(t.Context(), account, clearAt); err != nil {
		t.Fatal(err)
	}
	setAt := base.Add(2 * time.Second)
	backoff := routingentity.RetryBackoff{AccountID: account, UntilUnix: setAt.Add(time.Minute).Unix()}
	if err := store.SetRetryBackoff(t.Context(), backoff, setAt); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(store.root).ClearRetryBackoff(t.Context(), account, clearAt); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadRetryBackoffs(t.Context(), setAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].UntilUnix != backoff.UntilUnix {
		t.Fatalf("stale clear removed newer retry backoff: %+v", loaded)
	}
}
