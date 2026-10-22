package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

const geminiFailedSnapshotBefore04384 = "before<>&/🌍\u2028"
const geminiFailedSnapshotAfter04384 = "\tafter\n🌍\u2029"

func geminiFailedTextSnapshotCases04384() []string {
	return []string{"compact-owner-tool-error-text-finish", "compact-owner-tool-error-text-eof", "compact-owner-tool-repaired-text-finish", "compact-owner-tool-error-text-done"}
}

func geminiFailedTextSnapshotCase04384(scenario string) bool {
	return slices.Contains(geminiFailedTextSnapshotCases04384(), scenario)
}

func writeGeminiFailedTextSnapshot04384(writer io.Writer, scenario string) {
	repaired := scenario == "compact-owner-tool-repaired-text-finish"
	first := map[string]any{"created": 1, "model": "gemini-3.1-pro-preview", "choices": []any{}}
	if repaired {
		first["choices"] = []any{map[string]any{"delta": map[string]any{"content": geminiFailedSnapshotBefore04384, "tool_calls": []any{map[string]any{"index": 2, "id": "call_synthetic", "function": map[string]any{"arguments": "{}"}}}}, "finish_reason": "tool_calls"}}
	}
	writeGeminiLifecyclePacket04384(writer, first)
	if !repaired {
		writeGeminiLifecyclePacket04384(writer, map[string]any{"error": map[string]any{"code": "synthetic_error", "message": "synthetic semantic error"}})
	}
	function := map[string]any{"name": "lookup"}
	if !repaired {
		function["arguments"] = "{}"
	}
	call := map[string]any{"index": 2, "id": "call_synthetic", "function": function}
	delta := map[string]any{"content": geminiFailedSnapshotAfter04384, "tool_calls": []any{call}}
	writeGeminiLifecyclePacket04384(writer, map[string]any{"choices": []any{map[string]any{"delta": delta}}})
	if scenario != "compact-owner-tool-error-text-done" {
		writeGeminiLifecyclePacket04384(writer, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "tool_calls"}}})
	}
	if scenario != "compact-owner-tool-error-text-eof" {
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}
}

func geminiFailedTextSnapshotWire04384(scenario, responseID, itemID string) string {
	created := map[string]any{"type": "response.created", "created_at": 1, "response": map[string]any{"id": responseID}}
	failed := map[string]any{"type": "response.failed", "created_at": 1, "response": map[string]any{"id": responseID, "error": map[string]any{"code": "synthetic_error", "message": "synthetic semantic error"}}}
	textAdded := map[string]any{"type": "response.output_item.added", "response_id": responseID, "item": map[string]any{"id": itemID, "role": "assistant", "type": "message", "content": []any{}}}
	after := map[string]any{"type": "response.output_text.delta", "created_at": 1, "response_id": responseID, "delta": geminiFailedSnapshotAfter04384}
	toolAdded := map[string]any{"type": "response.output_item.added", "output_index": 2, "item": map[string]any{"type": "function_call", "call_id": "call_synthetic", "name": "lookup"}}
	arguments := map[string]any{"type": "response.function_call_arguments.delta", "output_index": 2, "call_id": "call_synthetic", "delta": "{}"}
	events := []map[string]any{created, failed, textAdded, after, toolAdded, arguments}
	if scenario == "compact-owner-tool-repaired-text-finish" {
		failed["response"].(map[string]any)["error"] = map[string]any{"code": "invalid_tool_call_arguments", "message": "Gemini OpenAI-compatible streamed a tool call without a function name at index 2"}
		toolAdded["item"].(map[string]any)["name"] = "tool_call"
		before := map[string]any{"type": "response.output_text.delta", "created_at": 1, "response_id": responseID, "delta": geminiFailedSnapshotBefore04384}
		events = []map[string]any{created, textAdded, before, toolAdded, arguments, failed, after}
	}
	if scenario != "compact-owner-tool-error-text-done" {
		events = append(events, map[string]any{"type": "response.output_item.done", "output_index": 2, "item": map[string]any{"type": "function_call", "call_id": "call_synthetic", "name": "lookup", "arguments": "{}"}})
	}
	var wire strings.Builder
	for sequence, event := range events {
		event["sequence_number"] = sequence
		encoded, _ := json.Marshal(event)
		canonical := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&", `\u2028`, "\u2028", `\u2029`, "\u2029").Replace(string(encoded))
		_, _ = fmt.Fprintf(&wire, "event: %s\r\ndata: %s\r\n\r\n", event["type"], canonical)
	}
	return wire.String()
}
