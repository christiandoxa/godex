package account

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepeatLoginPreservesNativeStateAndUpdatesAuth(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	home := store.CodexHome(account.ID)
	for _, name := range []string{"config.toml", "history.jsonl", "rollout-sentinel.jsonl"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("preserved native state"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	staged := stagedHome(t, store)
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte("replacement authentication"), 0600); err != nil {
		t.Fatal(err)
	}
	updated := newTestAccount(t, "work", "person@example.com", "account-1", time.Unix(2, 0))
	if _, err := store.CommitLogin(context.Background(), updated, staged, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.toml", "history.jsonl", "rollout-sentinel.jsonl"} {
		data, err := os.ReadFile(filepath.Join(home, name))
		if err != nil || string(data) != "preserved native state" {
			t.Fatalf("%s changed: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil || string(data) != "replacement authentication" {
		t.Fatalf("auth update failed: %v", err)
	}
}

func TestAuthenticationReplacementCanRollback(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	auth := filepath.Join(store.CodexHome(account.ID), "auth.json")
	old, err := os.ReadFile(auth)
	if err != nil {
		t.Fatal(err)
	}
	staged := stagedHome(t, store)
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	_, rollback, err := store.replaceAuthentication(account.ID, staged)
	if err != nil {
		t.Fatal(err)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(auth)
	if err != nil || string(restored) != string(old) {
		t.Fatalf("rollback failed: %v", err)
	}
}
