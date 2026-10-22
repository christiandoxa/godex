package chatcompat

import (
	"encoding/json"
	"testing"
)

func TestChatAssistantSnapshotKeepsSourceIndependent(t *testing.T) {
	function := map[string]any{"name": "shell", "arguments": `{"cmd":"ls"}`}
	call := map[string]any{"id": "call-synthetic", "function": function}
	message := map[string]any{"role": "user", "content": nil, "tool_calls": []any{call}, "provider_decoy": true}
	source := map[string]any{"choices": []any{map[string]any{"message": message}}}
	before, _ := json.Marshal(source)
	result := ChatAssistantMessages(source)
	encoded, _ := json.Marshal(result)
	want := `[{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"{\"cmd\":\"rtk ls\"}","name":"shell"},"id":"call-synthetic"}]}]`
	if string(encoded) != want {
		t.Fatalf("snapshot=%s, want %s", encoded, want)
	}
	after, _ := json.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("projection mutated the upstream object")
	}
	result[0].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["name"] = "changed"
	if function["name"] != "shell" {
		t.Fatal("snapshot aliases upstream calls")
	}
}

func TestChatAssistantSnapshotPreservesToolCallPresence(t *testing.T) {
	for _, fixture := range []struct {
		message map[string]any
		want    string
	}{
		{map[string]any{"content": ""}, `null`},
		{map[string]any{"content": false, "tool_calls": nil}, `[{"content":"","role":"assistant","tool_calls":null}]`},
		{map[string]any{"reasoning_content": "why"}, `[{"content":null,"reasoning_content":"why","role":"assistant"}]`},
	} {
		result := ChatAssistantMessages(map[string]any{"choices": []any{map[string]any{"message": fixture.message}}})
		body, err := json.Marshal(result)
		if err != nil || string(body) != fixture.want {
			t.Fatalf("projection=%s error=%v, want %s", body, err, fixture.want)
		}
	}
}
