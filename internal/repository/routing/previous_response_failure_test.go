package routing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestPreviousResponseFailuresPersistRouteScopedScoresAndDecay(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Unix(1_000_000, 0)
	responseKey := strings.Repeat("a", 64)
	accountID := strings.Repeat("1", 32)

	for range routingentity.PreviousResponseFailureThreshold {
		if _, err := store.RecordPreviousResponseFailure(context.Background(), accountID, responseKey, "responses", now); err != nil {
			t.Fatal(err)
		}
	}
	compact, err := store.RecordPreviousResponseFailure(context.Background(), accountID, responseKey, "compact", now)
	if err != nil || compact.Score != 1 {
		t.Fatalf("compact score = %#v, %v", compact, err)
	}

	failures, err := store.LoadPreviousResponseFailures(context.Background(), now.Add(180*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 2 {
		t.Fatalf("stored route scores = %#v", failures)
	}
	for _, failure := range failures {
		want := uint8(0)
		if failure.Route == "responses" {
			want = 1
		}
		if got := failure.Effective(now.Add(180 * time.Second)); got != want {
			t.Errorf("%s score after decay = %d, want %d", failure.Route, got, want)
		}
	}

	content, err := os.ReadFile(filepath.Join(store.root, "previous-response-failures.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "resp-secret") || !strings.Contains(string(content), responseKey) {
		t.Fatalf("persisted response key content = %s", content)
	}
}

func TestClearPreviousResponseFailuresRemovesOnlyMatchingAccount(t *testing.T) {
	store := NewStore(t.TempDir())
	key := strings.Repeat("b", 64)
	now := time.Unix(1_000_000, 0)
	firstAccount := strings.Repeat("2", 32)
	secondAccount := strings.Repeat("3", 32)
	for _, accountID := range []string{firstAccount, secondAccount} {
		for _, route := range []string{"responses", "compact"} {
			if _, err := store.RecordPreviousResponseFailure(context.Background(), accountID, key, route, now); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := store.ClearPreviousResponseFailures(context.Background(), firstAccount, key); err != nil {
		t.Fatal(err)
	}
	failures, err := store.LoadPreviousResponseFailures(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 2 || failures[0].AccountID != secondAccount || failures[1].AccountID != secondAccount {
		t.Fatalf("remaining failures = %#v", failures)
	}
}

func TestPreviousResponseFailureDecayedEntryRemainsKnownUntilRetention(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Unix(1_000_000, 0)
	accountID, key := strings.Repeat("4", 32), strings.Repeat("c", 64)
	if _, err := store.RecordPreviousResponseFailure(context.Background(), accountID, key, "responses", now); err != nil {
		t.Fatal(err)
	}
	failures, err := store.LoadPreviousResponseFailures(context.Background(), now.Add(routingentity.PreviousResponseFailureDecay))
	if err != nil || len(failures) != 1 || failures[0].Effective(now.Add(routingentity.PreviousResponseFailureDecay)) != 0 {
		t.Fatalf("expired failure marker = %#v, %v", failures, err)
	}
	failures, err = store.LoadPreviousResponseFailures(context.Background(), now.Add(routingentity.PreviousResponseFailureRetention))
	if err != nil || len(failures) != 0 {
		t.Fatalf("retired failure marker = %#v, %v", failures, err)
	}
}
