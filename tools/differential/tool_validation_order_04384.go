package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const toolValidationOrderError04384 = "DeepSeek streamed malformed JSON arguments for tool call `shell` at index 3: invalid JSON"

func toolValidationOrderCalls04384() []any {
	var calls []any
	for index := 18; index >= 0; index-- {
		if index == 1 || index == 2 {
			continue
		}
		arguments := "{"
		if index == 0 {
			arguments = `{}`
		}
		calls = append(calls, map[string]any{
			"index": index, "id": fmt.Sprintf("call-order-%d", index), "type": "function",
			"function": map[string]any{"name": "shell", "arguments": arguments},
		})
	}
	return calls
}

func toolValidationOrderUpstream04384(done bool) string {
	choice := map[string]any{"delta": map[string]any{"tool_calls": toolValidationOrderCalls04384()}}
	if !done {
		choice["finish_reason"] = "tool_calls"
	}
	chunk, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-tool-order", "model": "deepseek-v4-pro", "created": 1,
		"choices": []any{choice},
	})
	return "data: " + string(chunk) + "\n\ndata: [DONE]\n\n"
}

func toolValidationOrderWire04384() string {
	var wire strings.Builder
	sequence := 0
	write := func(name string, value map[string]any) {
		value["type"], value["sequence_number"] = name, sequence
		sequence++
		encoded, _ := json.Marshal(value)
		fmt.Fprintf(&wire, "event: %s\r\ndata: %s\r\n\r\n", name, encoded)
	}
	write("response.created", map[string]any{"created_at": 1, "response": map[string]any{"id": "chatcmpl-tool-order"}})
	for _, raw := range toolValidationOrderCalls04384() {
		call := raw.(map[string]any)
		write("response.output_item.added", map[string]any{
			"output_index": call["index"], "item": map[string]any{
				"type": "function_call", "name": "shell", "call_id": call["id"],
			},
		})
		write("response.function_call_arguments.delta", map[string]any{
			"output_index": call["index"], "call_id": call["id"],
			"delta": call["function"].(map[string]any)["arguments"],
		})
	}
	write("response.failed", map[string]any{
		"created_at": 1, "response": map[string]any{"id": "chatcmpl-tool-order", "error": map[string]any{
			"code": "invalid_tool_call_arguments", "message": toolValidationOrderError04384,
		}},
	})
	return wire.String()
}

func equivalentToolValidationOrderSSE04384(left, right string) bool {
	return left == right && left == toolValidationOrderWire04384()
}
