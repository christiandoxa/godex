package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Static, independent negative-control reference for the observed six-event
// Gemini Chat-compatible SSE life cycle. This function is intentionally not
// shared with the production Gemini gateway's SSE event builder.
func geminiChatSSEFixture043711(responseID, itemID string, epoch int64) string {
	output := []any{map[string]any{
		"type": "message", "role": "assistant",
		"content": []any{map[string]any{"type": "output_text", "text": "synthetic-ok"}},
	}}
	item := map[string]any{
		"id": itemID, "role": "assistant", "type": "message",
		"content": []any{map[string]any{"type": "output_text", "text": "synthetic-ok"}},
	}
	events := []map[string]any{
		{"type": "response.created", "created_at": epoch, "response": map[string]any{"id": responseID}},
		{"type": "response.output_item.added", "response_id": responseID, "item": map[string]any{
			"id": itemID, "role": "assistant", "type": "message", "content": []any{},
		}},
		{"type": "response.output_text.delta", "created_at": epoch, "response_id": responseID, "delta": "synthetic-"},
		{"type": "response.output_text.delta", "created_at": epoch, "response_id": responseID, "delta": "ok"},
		{"type": "response.output_item.done", "response_id": responseID, "item": item},
		{"type": "response.completed", "created_at": epoch, "response": map[string]any{
			"id": responseID, "model": "gemini-3.1-pro-preview", "output": output,
			"usage": map[string]any{
				"input_tokens": 2, "output_tokens": 1, "total_tokens": 3,
			},
			"metadata": map[string]any{
				"gemini": map[string]any{"finish_reason": "stop"},
			},
		}},
	}
	var result strings.Builder
	for index, event := range events {
		event["sequence_number"] = index
		body, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(&result, "event: %s\r\ndata: %s\r\n\r\n", event["type"], body)
	}
	return result.String()
}

func geminiChatSSESentinel043711() bool {
	good := geminiChatSSEFixture043711("resp_deepseek_fixture", "msg_gemini_fixture", 111)
	another := geminiChatSSEFixture043711("resp_deepseek_other", "msg_gemini_other", 999)
	if !geminiChatSSEEqual043711(good, another) {
		return false
	}
	for _, changed := range []string{
		strings.Replace(good, "\"sequence_number\":3", "\"sequence_number\":9", 1),
		strings.Replace(good, "\"response_id\":\"resp_deepseek_fixture\"", "\"response_id\":\"resp_other\"", 1),
		strings.Replace(good, "\"id\":\"msg_gemini_fixture\"", "\"id\":\"msg_other\"", 1),
		strings.Replace(good, "\"input_tokens\":2", "\"input_tokens\":7", 1),
		strings.Replace(good, "\"finish_reason\":\"stop\"", "\"finish_reason\":\"length\"", 1),
		strings.Replace(good, "\"delta\":\"ok\"", "\"delta\":\"forged\"", 1),
		strings.Replace(good, "event: response.output_item.done", "event: response.failed", 1),
		strings.Split(good, "event: response.completed")[0],
	} {
		if geminiChatSSEEqual043711(good, changed) {
			return false
		}
	}
	return true
}
