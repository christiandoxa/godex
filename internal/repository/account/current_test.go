package account

import (
	"context"
	"testing"
)

func TestCurrentTracksActiveAccountWithoutAdvancingRotation(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "one", "one@example.com", "account-1")
	second := commitTestAccount(t, store, "two", "two@example.com", "account-2")

	current, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != first.ID {
		t.Fatalf("initial current = %q", current.ID)
	}
	if _, err := store.SetActive(context.Background(), second.Name); err != nil {
		t.Fatal(err)
	}
	current, err = store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != second.ID {
		t.Fatalf("selected current = %q", current.ID)
	}
	selected, err := store.SelectForLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != second.ID {
		t.Fatalf("current lookup advanced rotation; selected %q", selected.ID)
	}
}

func TestCurrentRejectsEmptyState(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.Current(context.Background()); err == nil {
		t.Fatal("empty store unexpectedly has current account")
	}
}
