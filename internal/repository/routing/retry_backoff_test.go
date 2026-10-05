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
	if err := store.ClearRetryBackoff(t.Context(), account); err != nil {
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
