package main

import (
	"encoding/json"
)

// Exercise the independently authored, tagged multi-turn request contract:
// a tool result must follow its corresponding assistant call, even when
// the original history put the result later or in reversed call order.
func geminiMultiTurnSentinel043711() bool {
	fresh := func() []any {
		return []any{
			map[string]any{"role": "user", "content": "same request"},
			map[string]any{"role": "assistant", "content": "", "tool_calls": []any{
				map[string]any{"id": "call-a", "type": "function", "function": map[string]any{"name": "lookup", "arguments": "{}"}},
			}},
			map[string]any{"role": "tool", "tool_call_id": "call-a", "content": "first"},
			map[string]any{"role": "assistant", "content": "", "tool_calls": []any{
				map[string]any{"id": "call-b", "type": "function", "function": map[string]any{"name": "lookup", "arguments": "{}"}},
			}},
			map[string]any{"role": "tool", "tool_call_id": "call-b", "content": "second"},
			map[string]any{"role": "user", "content": "follow-up"},
		}
	}
	if err := geminiMultiTurnToolInvariant043711(fresh()); err != nil {
		return false
	}
	for _, mutate := range []func([]any){
		func(items []any) { items[2], items[5] = items[5], items[2] },
		func(items []any) { items[2].(map[string]any)["tool_call_id"] = "call-other" },
		func(items []any) {
			items[3].(map[string]any)["tool_calls"] = []any{
				map[string]any{"id": "call-b"}, map[string]any{"id": "orphan"},
			}
		},
		func(items []any) { items[4].(map[string]any)["content"] = "fabricated" },
		func(items []any) { items[1].(map[string]any)["content"] = "keep prose" },
		func(items []any) { items[5].(map[string]any)["content"] = "lost follow-up" },
	} {
		items := fresh()
		mutate(items)
		if err := geminiMultiTurnToolInvariant043711(items); err == nil {
			return false
		}
	}
	// JSON serialization must not cause the oracle to forget the tool
	// call ID or lose its paired textual output.
	encoded, err := json.Marshal(fresh())
	if err != nil {
		return false
	}
	var decoded []any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return false
	}
	return geminiMultiTurnToolInvariant043711(decoded) == nil
}
