package account

import (
	"context"
	"testing"
)

func TestLaunchCandidatesFollowRotationWithoutMutation(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "alpha", "alpha@example.com", "account-alpha")
	second := commitTestAccount(t, store, "beta", "beta@example.com", "account-beta")

	before, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := store.LaunchCandidates(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].ID != first.ID || candidates[1].ID != second.ID {
		t.Fatalf("candidates = %+v", candidates)
	}
	after, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID {
		t.Fatalf("candidate preview changed current account from %q to %q", before.ID, after.ID)
	}

	if _, err := store.SelectForLaunch(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	candidates, err = store.LaunchCandidates(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if candidates[0].ID != second.ID || candidates[1].ID != first.ID {
		t.Fatalf("rotated candidates = %+v", candidates)
	}
}

func TestLaunchCandidatesExplicitSelectorDoesNotRotate(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "alpha", "alpha@example.com", "account-alpha")
	second := commitTestAccount(t, store, "beta", "beta@example.com", "account-beta")
	candidates, err := store.LaunchCandidates(context.Background(), second.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ID != second.ID || candidates[0].ID == first.ID {
		t.Fatalf("explicit candidates = %+v", candidates)
	}
}
