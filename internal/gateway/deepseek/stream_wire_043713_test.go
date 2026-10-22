package deepseek

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

// An executable-vs-executable Prodex 0.437.13 SSE probe showed that the
// function_call's arguments first appear as deltas, not as a synthetic empty
// argument string inside response.output_item.added.
func TestProdex043713StreamedFunctionCallAddedHasNoArgumentsField(t *testing.T) {
	item := deepSeekStreamAddedToolItem("call-toolargs", "shell")
	if item == nil || item["type"] != "function_call" || item["call_id"] != "call-toolargs" || item["name"] != "shell" {
		t.Fatalf("unexpected streamed tool item: %#v", item)
	}
	if _, exists := item["arguments"]; exists {
		t.Fatalf("streamed added item introduced an arguments field before deltas: %#v", item)
	}
}

func TestProdex043713DeepSeekInvalidJSONFailureMatchesOfficialWireMessage(t *testing.T) {
	for _, arguments := range []string{`{"cmd":"\uD800"}`, `{"value":1e309}`, strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128)} {
		call := &deepSeekChatStreamToolCall{name: "shell"}
		call.arguments.WriteString(arguments)
		err := deepSeekValidateStreamToolCall(0, call)
		if err == nil || err.Error() != "DeepSeek streamed malformed JSON arguments for tool call `shell` at index 0: invalid JSON" {
			t.Fatalf("failure message differs from exact Prodex 0.437.13: %v", err)
		}
	}
	state := newDeepSeekChatStreamState(5, nil, nil, chatcompat.ConversationStore{})
	chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
		"delta": map[string]any{"tool_calls": []any{map[string]any{
			"index": 0, "id": "call-toolargs", "function": map[string]any{"name": "shell", "arguments": `{"cmd":"\uD800"}`},
		}}},
		"finish_reason": "tool_calls",
	}}})
	terminal, emitted, err := state.observe(chunk)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil || !emitted || !strings.Contains(string(terminal), `"message":"DeepSeek streamed malformed JSON arguments for tool call `+"`"+`shell`+"`"+` at index 0: invalid JSON"`) {
		t.Fatalf("invalid JSON stream did not emit canonical native failure: %q, %t, %v", terminal, emitted, err)
	}
	if late, supported, err := state.observe([]byte("[DONE]")); err != nil || supported || len(late) != 0 {
		t.Fatalf("DONE repeated a finished invalid-tool failure: %s supported=%v error=%v", late, supported, err)
	}
}
