package chatcompat

import (
	"fmt"
	"testing"
)

func TestConversationSnapshotsAreIndependent(t *testing.T) {
	store := NewConversationStore().Scoped("synthetic-session")
	message := map[string]any{"role": "assistant", "tool_calls": []any{
		map[string]any{"id": "call-synthetic", "function": map[string]any{"name": "read"}},
	}}
	store.Insert("resp-synthetic", []any{message})
	message["role"] = "user"
	history := store.History("resp-synthetic")
	history[0].(map[string]any)["role"] = "system"
	byCall := store.FindHistoryByCallID("call-synthetic")
	if byCall[0].(map[string]any)["role"] != "assistant" {
		t.Fatalf("snapshot mutated: %#v", byCall)
	}
	byCall[0].(map[string]any)["tool_calls"] = nil
	if len(store.FindHistoryByCallID("call-synthetic")) != 1 {
		t.Fatal("call lookup returned mutable stored messages")
	}
}

func TestConversationStoreCapsAllScopes(t *testing.T) {
	store := NewConversationStore()
	for index := 0; index <= conversationsTotal; index++ {
		store.Scoped(fmt.Sprintf("session-%d", index)).Insert("resp-synthetic", []any{"context"})
	}
	if store.Scoped("session-0").Contains("resp-synthetic") {
		t.Fatal("oldest conversation survived global cap")
	}
	if !store.Scoped(fmt.Sprintf("session-%d", conversationsTotal)).Contains("resp-synthetic") {
		t.Fatal("newest conversation was evicted")
	}
}
