package gemini

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestProdex043711GeminiMultiTurnToolAdjacencyMatchesMojo(t *testing.T) {
	call := func(id string) map[string]any {
		return map[string]any{
			"id": id, "type": "function",
			"function": map[string]any{"name": "lookup", "arguments": "{}"},
		}
	}
	for _, fixture := range []struct {
		name  string
		input []any
		want  []any
	}{
		{
			name: "completed tool output moves before interposed user, orphan call removed and assistant prose retained",
			input: []any{
				map[string]any{"role": "user", "content": "first"},
				map[string]any{"role": "assistant", "content": "keep prose", "tool_calls": []any{call("call-1"), call("orphan")}},
				map[string]any{"role": "user", "content": "later"},
				map[string]any{"role": "tool", "tool_call_id": "call-1", "content": "ok"},
			},
			want: []any{
				map[string]any{"role": "user", "content": "first"},
				map[string]any{"role": "assistant", "content": "", "tool_calls": []any{call("call-1")}},
				map[string]any{"role": "tool", "tool_call_id": "call-1", "content": "ok"},
				map[string]any{"role": "assistant", "content": "keep prose"},
				map[string]any{"role": "user", "content": "later"},
			},
		},
		{
			name: "two completed tools are paired in call order despite reversed outputs",
			input: []any{
				map[string]any{"role": "assistant", "tool_calls": []any{call("call-a"), call("call-b")}},
				map[string]any{"role": "user", "content": "after"},
				map[string]any{"role": "tool", "tool_call_id": "call-b", "content": "second"},
				map[string]any{"role": "tool", "tool_call_id": "call-a", "content": "first"},
			},
			want: []any{
				map[string]any{"role": "assistant", "content": "", "tool_calls": []any{call("call-a")}},
				map[string]any{"role": "tool", "tool_call_id": "call-a", "content": "first"},
				map[string]any{"role": "assistant", "content": "", "tool_calls": []any{call("call-b")}},
				map[string]any{"role": "tool", "tool_call_id": "call-b", "content": "second"},
				map[string]any{"role": "user", "content": "after"},
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			request := map[string]any{"model": "gemini-3.1-pro-preview", "messages": fixture.input, "stream": false}
			encoded, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			got, err := filterGeminiOrphanToolMessages043711(encoded)
			if err != nil {
				t.Fatal(err)
			}
			var parsed map[string]any
			if err := json.Unmarshal(got, &parsed); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(parsed["messages"], fixture.want) {
				t.Fatalf("tagged 0.437.11 multi-turn Gemini adjacency got=%+v want=%+v", parsed["messages"], fixture.want)
			}
		})
	}
}

func TestProdex043711GeminiAdjacencyConsumesEarlyDuplicateOutputOnlyOnce(t *testing.T) {
	call := func(id string) map[string]any {
		return map[string]any{
			"id": id, "type": "function", "function": map[string]any{"name": "lookup", "arguments": "{}"},
		}
	}
	request := map[string]any{"model": "gemini-3.1-pro-preview", "messages": []any{
		map[string]any{"role": "tool", "tool_call_id": "a", "content": "first"},
		map[string]any{"role": "tool", "tool_call_id": "a", "content": "duplicate"},
		map[string]any{"role": "assistant", "tool_calls": []any{call("a"), call("a"), call("missing")}},
		map[string]any{"role": "assistant", "tool_calls": []any{call("a")}, "content": "keep prose"},
		map[string]any{"role": "user", "tool_calls": []any{call("b")}},
		map[string]any{"role": "tool", "tool_call_id": "b", "content": "second result"},
	}}
	expected := []any{
		map[string]any{"role": "assistant", "tool_calls": []any{call("a")}, "content": ""},
		map[string]any{"role": "tool", "tool_call_id": "a", "content": "first"},
		map[string]any{"role": "assistant", "content": "keep prose"},
		map[string]any{"role": "user", "tool_calls": []any{call("b")}},
		map[string]any{"role": "tool", "tool_call_id": "b", "content": "second result"},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	translated, err := filterGeminiOrphanToolMessages043711(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]any
	if err := json.Unmarshal(translated, &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual["messages"], expected) {
		t.Fatalf("tagged 0.437.11 first output and duplicate handling mismatch: got=%+v want=%+v", actual["messages"], expected)
	}
}
