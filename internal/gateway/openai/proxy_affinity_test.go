package openai

import (
	"reflect"
	"testing"
	"time"
)

func TestAffinityExpiresAndRejectsConflicts(t *testing.T) {
	store := newAffinityStore()
	now := time.Unix(100, 0)
	keys := affinityKeys{previous: "response-1", turn: "turn-1", session: "session-1"}
	if err := store.remember("account-a", keys, now); err != nil {
		t.Fatal(err)
	}
	if owner, err := store.owner(keys, now.Add(affinityTTL-time.Second)); err != nil || owner != "account-a" {
		t.Fatalf("owner before expiry = %q, %v", owner, err)
	}
	if owner, err := store.owner(keys, now.Add(affinityTTL)); err != nil || owner != "" {
		t.Fatalf("owner at expiry = %q, %v", owner, err)
	}
	if err := store.remember("account-a", keys, now); err != nil {
		t.Fatal(err)
	}
	if err := store.remember("account-b", affinityKeys{previous: "response-1"}, now); err == nil {
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
