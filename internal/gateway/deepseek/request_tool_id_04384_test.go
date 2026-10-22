package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekToolAdjacencyKeepsExactIDs(t *testing.T) {
	for _, fixture := range []struct {
		name, callID, outputID string
		paired                 bool
	}{
		{"exact", "call_synthetic", "call_synthetic", true},
		{"same padding", " \tcall_synthetic\u2003", " \tcall_synthetic\u2003", true},
		{"padded call", " call_synthetic ", "call_synthetic", false},
		{"padded output", "call_synthetic", " call_synthetic ", false},
		{"Unicode call", "\u2003call_synthetic\u2003", "call_synthetic", false},
		{"different padding", " call_synthetic", "call_synthetic ", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			input := map[string]any{"input": []any{
				map[string]any{"type": "message", "role": "developer", "content": "synthetic instruction"},
				map[string]any{"type": "function_call", "call_id": fixture.callID, "name": "lookup", "arguments": "{}"},
				map[string]any{"type": "function_call_output", "call_id": fixture.outputID, "output": "done"},
				map[string]any{"type": "message", "role": "user", "content": "same request"},
			}}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			body, err = ResponsesRequest(body, RequestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var request map[string]any
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			messages := request["messages"].([]any)
			count := 2
			if fixture.paired {
				count = 4
			}
			if len(messages) != count {
				t.Fatal("tool matching merged distinct raw IDs or lost an exact padded ID")
			}
			if fixture.paired {
				call := messages[1].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
				if call["id"] != fixture.callID || messages[2].(map[string]any)["tool_call_id"] != fixture.outputID {
					t.Fatal("matching changed raw call identity")
				}
			}
		})
	}
}

func TestProdex04384DeepSeekToolAdjacencyKeepsDistinctPaddingPairs(t *testing.T) {
	first := assistantToolCall("call_synthetic", "first_lookup", "{}")
	second := assistantToolCall(" call_synthetic ", "second_lookup", "{}")
	firstOutput := map[string]any{"role": "tool", "tool_call_id": "call_synthetic", "content": "first"}
	secondOutput := map[string]any{"role": "tool", "tool_call_id": " call_synthetic ", "content": "second"}
	messages := repairDeepSeekToolCallAdjacency([]any{first, second, secondOutput, firstOutput})
	if len(messages) != 4 || messages[1].(map[string]any)["content"] != "first" || messages[3].(map[string]any)["content"] != "second" {
		t.Fatal("distinct padded IDs shared first-output or emitted-call ownership")
	}
}
