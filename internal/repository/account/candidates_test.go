package account

import (
	"context"
	"strings"
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

func TestProdex04356LaunchCandidatesExcludeSelectedAPIKeyProfiles(t *testing.T) {
	store := newTestStore(t)
	api := commitTestAccount(t, store, "api", "api@example.com", "account-api")
	chatgpt := commitTestAccount(t, store, "chatgpt", "chatgpt@example.com", "account-chatgpt")
	if _, err := store.ApplySelectedAPIKey(
		t.Context(), api.ID,
		[]byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"<redacted>"}`), nil, nil,
	); err != nil {
		t.Fatal(err)
	}

	candidates, err := store.LaunchCandidates(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ID != chatgpt.ID {
		t.Fatalf("quota-compatible candidates = %#v", candidates)
	}
	if _, err := store.LaunchCandidates(t.Context(), api.Name); err == nil || !strings.Contains(err.Error(), "not ChatGPT quota-compatible") {
		t.Fatalf("explicit API-key legacy selector error = %v", err)
	}
	for i := 0; i < 3; i++ {
		selected, err := store.SelectForLaunch(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		if selected.ID != chatgpt.ID {
			t.Fatalf("selection %d chose API-key profile: %#v", i, selected)
		}
	}
}

func TestProdex04356LaunchCandidatesFailWhenOnlyAPIKeyProfilesRemain(t *testing.T) {
	store := newTestStore(t)
	api := commitTestAccount(t, store, "api", "api@example.com", "account-api")
	if _, err := store.ApplySelectedAPIKey(
		t.Context(), api.ID,
		[]byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"<redacted>"}`), nil, nil,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LaunchCandidates(t.Context(), ""); err == nil || !strings.Contains(err.Error(), "no enabled accounts") {
		t.Fatalf("only API-key candidate error = %v", err)
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
