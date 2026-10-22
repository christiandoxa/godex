package main

import "encoding/json"

func deepSeekToolIDSuffixes04384() []string {
	return []string{"tool-id-exact", "tool-id-padded-both", "tool-id-padded-call", "tool-id-padded-output", "tool-id-unicode-call", "tool-id-distinct-padding", "tool-id-distinct-pairs", "tool-id-simple-unmatched"}
}

func deepSeekToolIDFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	callID, outputID := "call_synthetic", "call_synthetic"
	paired := true
	switch suffix {
	case "tool-id-exact":
	case "tool-id-padded-both":
		callID, outputID = " \tcall_synthetic\u2003", " \tcall_synthetic\u2003"
	case "tool-id-padded-call", "tool-id-simple-unmatched":
		callID, paired = " call_synthetic ", false
	case "tool-id-padded-output":
		outputID, paired = " call_synthetic ", false
	case "tool-id-unicode-call":
		callID, paired = "\u2003call_synthetic\u2003", false
	case "tool-id-distinct-padding":
		callID, outputID, paired = " call_synthetic", "call_synthetic ", false
	case "tool-id-distinct-pairs":
	default:
		return requestPrimitiveFixture04384{}, false
	}
	call := map[string]any{"type": "function_call", "call_id": callID, "name": "lookup", "arguments": "{}"}
	output := map[string]any{"type": "function_call_output", "call_id": outputID, "output": "done"}
	input := []any{map[string]any{"type": "message", "role": "developer", "content": "synthetic instruction"}, call, output}
	messages := []any{map[string]any{"role": "system", "content": "synthetic instruction"}}
	pair := []any{
		map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": callID, "type": "function", "function": map[string]any{"name": "lookup", "arguments": "{}"}}}},
		map[string]any{"role": "tool", "tool_call_id": outputID, "content": "done"},
	}
	if suffix == "tool-id-simple-unmatched" {
		input, messages, paired = input[1:], nil, true
	}
	if paired {
		messages = append(messages, pair...)
	}
	if suffix == "tool-id-distinct-pairs" {
		input = append(input[:2], map[string]any{"type": "function_call", "call_id": " call_synthetic ", "name": "other_lookup", "arguments": "{}"}, map[string]any{"type": "function_call_output", "call_id": " call_synthetic ", "output": "second"}, output)
		messages = append(messages, map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": " call_synthetic ", "type": "function", "function": map[string]any{"name": "other_lookup", "arguments": "{}"}}}}, map[string]any{"role": "tool", "tool_call_id": " call_synthetic ", "content": "second"})
	}
	input = append(input, map[string]any{"type": "message", "role": "user", "content": "same request"})
	messages = append(messages, map[string]any{"role": "user", "content": "same request"})
	body, _ := json.Marshal(map[string]any{"model": "gemini-3.1-pro-preview", "input": input})
	wire, _ := json.Marshal(map[string]any{"messages": messages, "model": "gemini-3.1-pro-preview", "stream": false})
	return requestPrimitiveFixture04384{body: string(body), wireBody: string(wire)}, true
}
