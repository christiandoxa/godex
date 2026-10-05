package routing

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAffinityExpiresAndRejectsConflicts(t *testing.T) {
	store := newAffinityStore()
	now := time.Unix(100, 0)
	keys := affinityKeys{previous: "response-1", turn: "turn-1", session: "session-1"}
	if err := store.remember(context.Background(), "account-a", keys, now); err != nil {
		t.Fatal(err)
	}
	if owner, err := store.owner(context.Background(), keys, now.Add(affinityTTL-time.Second)); err != nil || owner != "account-a" {
		t.Fatalf("owner before expiry = %q, %v", owner, err)
	}
	if owner, err := store.owner(context.Background(), keys, now.Add(affinityTTL)); err != nil || owner != "" {
		t.Fatalf("owner at expiry = %q, %v", owner, err)
	}
	if err := store.remember(context.Background(), "account-a", keys, now); err != nil {
		t.Fatal(err)
	}
	if err := store.remember(context.Background(), "account-b", affinityKeys{previous: "response-1"}, now); err == nil {
		t.Fatal("expected affinity conflict")
	}
}

func TestResponseAffinityReadsNestedResponseIdentifiers(t *testing.T) {
	keys := responseObjectAffinity(map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         "response-nested",
			"session_id": "session-nested",
			"turn_state": "turn-nested",
		},
	})
	want := affinityKeys{previous: "response-nested", session: "session-nested", turn: "turn-nested"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("nested affinity = %#v, want %#v", keys, want)
	}
}

func TestResponseTurnStateIsOwnerBoundAndExpires(t *testing.T) {
	store := newAffinityStore()
	now := time.Unix(100, 0)
	store.rememberResponseTurnState("resp_owner", "account-a", "turn-owner", now)
	if got := store.responseTurnState("resp_owner", "account-b", now); got != "" {
		t.Fatalf("turn state for another account = %q", got)
	}
	if got := store.responseTurnState("resp_owner", "account-a", now); got != "turn-owner" {
		t.Fatalf("turn state = %q, want turn-owner", got)
	}
	if got := store.responseTurnState("resp_owner", "account-a", now.Add(affinityTTL)); got != "" {
		t.Fatalf("turn state at expiry = %q", got)
	}
}

func TestResponseTurnStateRejectsOversizedValues(t *testing.T) {
	store := newAffinityStore()
	store.rememberResponseTurnState("resp_owner", "account-a", strings.Repeat("x", maxAffinityValue+1), time.Now())
	if got := store.responseTurnState("resp_owner", "account-a", time.Now()); got != "" {
		t.Fatal("oversized turn state was retained")
	}
}
