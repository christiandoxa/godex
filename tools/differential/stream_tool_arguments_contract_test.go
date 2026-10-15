package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func testProdex043713StreamToolFixture(t *testing.T, scenario string) string {
	t.Helper()
	argument, valid, known := toolArgumentScenario043713(scenario)
	if !known {
		t.Fatalf("unknown scenario %q", scenario)
	}
	records := []struct {
		name string
		data map[string]any
	}{
		{"response.created", map[string]any{"type": "response.created", "sequence_number": 0, "created_at": 1, "response": map[string]any{"id": "chatcmpl-toolargs"}}},
		{"response.output_item.added", map[string]any{"type": "response.output_item.added", "sequence_number": 1, "output_index": 0, "item": map[string]any{"type": "function_call", "call_id": "call-toolargs", "name": "shell"}}},
		{"response.function_call_arguments.delta", map[string]any{"type": "response.function_call_arguments.delta", "sequence_number": 2, "output_index": 0, "call_id": "call-toolargs", "delta": argument}},
	}
	if valid {
		item := map[string]any{"type": "function_call", "call_id": "call-toolargs", "name": "shell", "arguments": argument}
		records = append(records,
			struct {
				name string
				data map[string]any
			}{"response.output_item.done", map[string]any{"type": "response.output_item.done", "sequence_number": 3, "output_index": 0, "item": item}},
			struct {
				name string
				data map[string]any
			}{"response.completed", map[string]any{"type": "response.completed", "sequence_number": 4, "created_at": 1, "response": map[string]any{"id": "chatcmpl-toolargs", "model": "deepseek-v4-pro", "metadata": map[string]any{"deepseek": map[string]any{"finish_reason": "tool_calls"}}, "output": []any{item}}}},
		)
	} else {
		records = append(records, struct {
			name string
			data map[string]any
		}{"response.failed", map[string]any{
			"type": "response.failed", "sequence_number": 3, "created_at": 1,
			"response": map[string]any{"id": "chatcmpl-toolargs", "error": map[string]any{
				"code": "invalid_tool_call_arguments", "message": "DeepSeek streamed malformed JSON arguments for tool call `shell` at index 0: invalid JSON",
			}},
		}})
	}
	var result strings.Builder
	for _, record := range records {
		data, err := json.Marshal(record.data)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&result, "event: %s\r\ndata: %s\r\n\r\n", record.name, data)
	}
	return result.String()
}

func TestProdex043713StreamToolDifferentialFixtureRejectsMutations(t *testing.T) {
	for _, scenario := range []string{"sse-tool-surrogate", "sse-tool-depth", "sse-tool-overflow", "sse-tool-paired"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := testProdex043713StreamToolFixture(t, scenario)
			if !validFixtureToolArgumentsSSE043713(fixture, scenario) || !equivalentFixtureToolArgumentsSSE043713(fixture, fixture) {
				t.Fatalf("correct reference fixture rejected: %s", scenario)
			}
			wrong := "sse-tool-paired"
			if scenario == wrong {
				wrong = "sse-tool-depth"
			}
			if validFixtureToolArgumentsSSE043713(fixture, wrong) {
				t.Fatal("different case was accepted")
			}
			for _, mutation := range []struct{ name, from, to string }{
				{"missing origin", "chatcmpl-toolargs", "another-id"},
				{"wrong sequence", `"sequence_number":2`, `"sequence_number":99`},
				{"stale added arguments", `"call_id":"call-toolargs","name":"shell","type":"function_call"`, `"arguments":"","call_id":"call-toolargs","name":"shell","type":"function_call"`},
				{"missing delta", `"output_index":0,"sequence_number":2`, `"output_index":3,"sequence_number":2`},
				{"corrupted error", `invalid JSON`, `bad parser`},
			} {
				if !strings.Contains(fixture, mutation.from) {
					if mutation.name == "corrupted error" && scenario == "sse-tool-paired" {
						continue
					}
					t.Fatalf("missing mutation marker %q", mutation.from)
				}
				corrupted := strings.Replace(fixture, mutation.from, mutation.to, 1)
				if validFixtureToolArgumentsSSE043713(corrupted, scenario) || equivalentFixtureToolArgumentsSSE043713(fixture, corrupted) {
					t.Fatalf("mutation %q evaded semantic sentinel", mutation.name)
				}
			}
		})
	}
}
