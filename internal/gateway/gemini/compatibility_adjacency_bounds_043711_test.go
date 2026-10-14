package gemini

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The tagged 0.437.11 Mojo adjacency kernel qualifies 2048 assistant
// tool calls paired with outputs received in reverse order. This prevents
// an implementation with quadratic scans or a stale call/output cursor
// from silently passing only the two-tool synthetic differential.
func TestProdex043711GeminiAdjacencyHandles2048ReverseOrderToolResults(t *testing.T) {
	const pairCount = 2048
	messages := make([]any, 0, pairCount*2)
	for index := range pairCount {
		id := fmt.Sprintf("call_%05d", index)
		messages = append(messages, map[string]any{
			"role":       "assistant",
			"tool_calls": []any{map[string]any{"id": id}},
		})
	}
	for index := pairCount - 1; index >= 0; index-- {
		id := fmt.Sprintf("call_%05d", index)
		messages = append(messages, map[string]any{
			"role": "tool", "tool_call_id": id,
			"content": fmt.Sprintf("output %d", index),
		})
	}
	request, err := json.Marshal(map[string]any{"messages": messages})
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := filterGeminiOrphanToolMessages043711(request)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(rewritten, &parsed); err != nil {
		t.Fatal(err)
	}
	pairs, _ := parsed["messages"].([]any)
	if len(pairs) != pairCount*2 {
		t.Fatalf("tagged Mojo 2048-call adjacency emitted %d messages, want %d", len(pairs), pairCount*2)
	}
	for index := range pairCount {
		assistant := pairs[index*2].(map[string]any)
		calls := assistant["tool_calls"].([]any)
		if assistant["role"] != "assistant" || assistant["content"] != "" || len(calls) != 1 {
			t.Fatalf("assistant %d content/calls changed: %+v", index, assistant)
		}
		id := fmt.Sprintf("call_%05d", index)
		if calls[0].(map[string]any)["id"] != id {
			t.Fatalf("assistant %d wrong tool id: %+v", index, calls[0])
		}
		result := pairs[index*2+1].(map[string]any)
		if result["role"] != "tool" || result["tool_call_id"] != id ||
			result["content"] != fmt.Sprintf("output %d", index) {
			t.Fatalf("tool %d result got %+v", index, result)
		}
	}
}

func TestProdex043711GeminiAdjacencyRetainsMalformedAndUnicodeContent(t *testing.T) {
	contents := []any{
		map[string]any{"role": "tool", "tool_call_id": " ", "content": "unmatched"},
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "a"}}, "content": false},
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "b"}}, "reasoning_content": []any{nil}},
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "c"}}, "content": "\u001c"},
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "d"}}, "content": "\u2003"},
	}
	encoded, err := json.Marshal(map[string]any{"messages": contents})
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := filterGeminiOrphanToolMessages043711(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(rewritten, &root); err != nil {
		t.Fatal(err)
	}
	expected := []any{
		map[string]any{"role": "tool", "tool_call_id": " ", "content": "unmatched"},
		map[string]any{"role": "assistant", "content": false},
		map[string]any{"role": "assistant", "reasoning_content": []any{nil}},
		map[string]any{"role": "assistant", "content": "\u001c"},
	}
	if !reflect.DeepEqual(root["messages"], expected) {
		t.Fatalf("tagged Mojo malformed and Unicode blankness mismatch: got=%+v want=%+v", root["messages"], expected)
	}
	// Preserve the distinction: U+001C is visible for this source policy,
	// while U+2003 is Unicode whitespace and must not turn into a call.
	if strings.TrimSpace("\u001c") == "" || strings.TrimSpace("\u2003") != "" {
		t.Fatal("underlying Unicode whitespace classification changed")
	}
}
