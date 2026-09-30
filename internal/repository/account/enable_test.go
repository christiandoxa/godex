package account

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDisableRetainsHomeAndRepairsSelection(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "one", "one@example.com", "account-1")
	second := commitTestAccount(t, store, "two", "two@example.com", "account-2")
	if _, err := store.SetEnabled(context.Background(), first.ID, false); err != nil {
		t.Fatal(err)
	}
	current, err := store.Current(context.Background())
	if err != nil || current.ID != second.ID {
		t.Fatalf("current = %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.CodexHome(first.ID), "auth.json")); err != nil {
		t.Fatal("disabled profile lost native state")
	}
	candidates, err := store.LaunchCandidates(context.Background(), "")
	if err != nil || len(candidates) != 1 || candidates[0].ID != second.ID {
		t.Fatalf("candidates = %v, %v", candidates, err)
	}
	if _, err := store.SetEnabled(context.Background(), first.ID, true); err != nil {
		t.Fatal(err)
	}
	candidates, err = store.LaunchCandidates(context.Background(), "")
	if err != nil || len(candidates) != 2 {
		t.Fatalf("enabled candidates = %v", err)
	}
}
