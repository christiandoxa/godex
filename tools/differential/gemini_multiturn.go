package main

import (
	"errors"
	"fmt"
)

// The tagged 0.437.11 chat-compatible bridge repairs the entire response
// history (not just tool-call presence). Two completed tool calls must each
// precede their matching output, ordered by the original call sequence.
// The intervening user text remains after the completed pairs, and the
// unanswered call is removed.
func geminiMultiTurnToolInvariant043711(messages []any) error {
	roles := []string{"user", "assistant", "tool", "assistant", "tool", "user"}
	if len(messages) != len(roles) {
		return fmt.Errorf("tagged Gemini multi-turn adjacency emitted %d messages, want %d: %+v",
			len(messages), len(roles), messages)
	}
	for i, wanted := range roles {
		entry, ok := messages[i].(map[string]any)
		if !ok || entry["role"] != wanted {
			return fmt.Errorf("Gemini multi-turn role/order %d changed: %+v", i, messages)
		}
	}
	first, _ := messages[0].(map[string]any)
	last, _ := messages[5].(map[string]any)
	if first["content"] != "same request" || last["content"] != "follow-up" {
		return fmt.Errorf("Gemini multi-turn user prose lost: %+v", messages)
	}
	for index, expectedID := range []string{"call-a", "call-b"} {
		assistant, _ := messages[1+index*2].(map[string]any)
		tool, _ := messages[2+index*2].(map[string]any)
		calls, _ := assistant["tool_calls"].([]any)
		if len(calls) != 1 || assistant["content"] != "" {
			return fmt.Errorf("Gemini multi-turn assistant call %d malformed: %+v", index, assistant)
		}
		call, _ := calls[0].(map[string]any)
		if call["id"] != expectedID || tool["tool_call_id"] != expectedID {
			return fmt.Errorf("Gemini multi-turn tool adjacency invalid for %s: %+v", expectedID, messages)
		}
		function, _ := call["function"].(map[string]any)
		if function["name"] != "lookup" {
			return errors.New("Gemini multi-turn tool name changed")
		}
		output := "first"
		if index == 1 {
			output = "second"
		}
		if tool["content"] != output {
			return fmt.Errorf("Gemini multi-turn tool %s output changed: %+v", expectedID, tool)
		}
	}
	return nil
}
