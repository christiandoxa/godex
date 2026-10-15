package routing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestProdex043714RestartAuthBackoffRequiresCausalClearAndTombstone(t *testing.T) {
	now := time.Unix(1700000000, 0)
	ctx := context.Background()
	store := NewStore(t.TempDir())
	failure := routingentity.AuthBackoff{AccountID: "profile-one", FailedAtMillis: now.UnixMilli(), UntilMillis: now.Add(time.Hour).UnixMilli()}
	if err := store.SetAuthBackoff(ctx, failure, now); err != nil {
		t.Fatal(err)
	}
	entries, err := NewStore(store.root).LoadAuthBackoffs(ctx, now)
	if err != nil || len(entries) != 1 || entries[0] != failure {
		t.Fatalf("penalty did not survive restart: %+v %v", entries, err)
	}
	if removed, err := store.ClearAuthBackoff(ctx, failure.AccountID, failure.FailedAtMillis-1, now); err != nil || removed {
		t.Fatalf("stale receipt cleared newer 401: %t %v", removed, err)
	}
	if removed, err := store.ClearAuthBackoff(ctx, failure.AccountID, failure.FailedAtMillis, now.Add(time.Second)); err != nil || !removed {
		t.Fatalf("authenticated receipt did not clear exact failure: %t %v", removed, err)
	}
	if err := store.SetAuthBackoff(ctx, failure, now); err != nil {
		t.Fatal(err)
	}
	entries, err = NewStore(store.root).LoadAuthBackoffs(ctx, now)
	if err != nil || len(entries) != 0 {
		t.Fatalf("late old writer resurrected 401: %+v %v", entries, err)
	}
	newer := routingentity.AuthBackoff{AccountID: failure.AccountID, FailedAtMillis: now.Add(2 * time.Second).UnixMilli(), UntilMillis: now.Add(time.Hour).UnixMilli()}
	if err := store.SetAuthBackoff(ctx, newer, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.ClearAuthBackoff(ctx, failure.AccountID, failure.FailedAtMillis, now.Add(3*time.Second)); err != nil || removed {
		t.Fatalf("old receipt removed new auth failure: %t %v", removed, err)
	}
	entries, err = NewStore(store.root).LoadAuthBackoffs(ctx, now.Add(3*time.Second))
	if err != nil || len(entries) != 1 || entries[0] != newer {
		t.Fatalf("newer auth penalty not preserved: %+v %v", entries, err)
	}
}

func TestProdex043714RestartAuthBackoffRejectsCorruptAndPublicState(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Unix(1700000000, 0)
	entry := routingentity.AuthBackoff{AccountID: "profile-one", FailedAtMillis: now.UnixMilli(), UntilMillis: now.Add(time.Minute).UnixMilli()}
	if err := store.SetAuthBackoff(t.Context(), entry, now); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "auth-backoff.json")
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadAuthBackoffs(t.Context(), now); err != nil {
		t.Fatal("private backup should recover unsafe primary: ", err)
	}
	if err := os.WriteFile(file, []byte(`{"version":1,"backoffs":[{"account_id":"profile-one","failed_at_ms":-1,"until_ms":0}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadAuthBackoffs(t.Context(), now); err != nil {
		t.Fatal("valid private backup should recover corrupted primary: ", err)
	}
	if err := os.WriteFile(file, []byte(`{"version":1,"backoffs":[{"account_id":"profile-one","failed_at_ms":-1,"until_ms":0}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file+".last-good", []byte(`{"version":1,"backoffs":[{"account_id":"profile-one","failed_at_ms":-1,"until_ms":0}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadAuthBackoffs(t.Context(), now); err == nil {
		t.Fatal("corrupt primary and backup silently accepted")
	}
}
