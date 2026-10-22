package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

func geminiRepairedSnapshotCases04384() []string {
	return []string{"compact-owner-tool-repaired-name", "compact-owner-tool-error-valid-finish", "compact-owner-tool-error-valid-done"}
}

func geminiRepairedSnapshotCase04384(scenario string) bool {
	return slices.Contains(geminiRepairedSnapshotCases04384(), scenario)
}

func writeGeminiRepairedSnapshot04384(writer io.Writer, scenario string) {
	if scenario == "compact-owner-tool-repaired-name" {
		writeGeminiLifecyclePacket04384(writer, map[string]any{"created": 1, "model": "gemini-3.1-pro-preview", "choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 2, "id": "call_synthetic", "function": map[string]any{"arguments": "{}"}}}}}}})
		writeGeminiLifecyclePacket04384(writer, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "tool_calls"}}})
		writeGeminiLifecyclePacket04384(writer, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 2, "function": map[string]any{"name": "lookup"}}}}}}})
		writeGeminiLifecyclePacket04384(writer, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "tool_calls"}}})
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
		return
	}
	writeGeminiLifecyclePacket04384(writer, map[string]any{"created": 1, "model": "gemini-3.1-pro-preview", "choices": []any{}})
	writeGeminiLifecyclePacket04384(writer, map[string]any{"error": map[string]any{"code": "synthetic_error", "message": "synthetic semantic error"}})
	var wire strings.Builder
	writeGeminiToolLifecycle04384(&wire, "stream-chat-tool-simple")
	for _, block := range strings.Split(strings.TrimSuffix(wire.String(), "\n\n"), "\n\n") {
		if scenario == "compact-owner-tool-error-valid-done" && strings.Contains(block, `"finish_reason"`) {
			continue
		}
		_, _ = io.WriteString(writer, block+"\n\n")
	}
}

func geminiRepairedSnapshotSeedWire04384(scenario, responseID string) string {
	parts := strings.Split(geminiLifecycleWire04384("stream-chat-tool-simple", responseID, ""), "\r\n\r\n")
	if scenario == "compact-owner-tool-repaired-name" {
		failed := strings.Split(geminiLifecycleWire04384("stream-chat-tool-missing-name", responseID, ""), "\r\n\r\n")
		return strings.Join(failed[:4], "\r\n\r\n") + "\r\n\r\n" + strings.Replace(parts[3], `"sequence_number":3`, `"sequence_number":4`, 1) + "\r\n\r\n"
	}
	failed := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":1,\"response\":{\"error\":{\"code\":\"synthetic_error\",\"message\":\"synthetic semantic error\"},\"id\":%q},\"sequence_number\":1,\"type\":\"response.failed\"}", responseID)
	events := []string{parts[0], failed, strings.Replace(parts[1], `"sequence_number":1`, `"sequence_number":2`, 1), strings.Replace(parts[2], `"sequence_number":2`, `"sequence_number":3`, 1)}
	if scenario == "compact-owner-tool-error-valid-finish" {
		events = append(events, strings.Replace(parts[3], `"sequence_number":3`, `"sequence_number":4`, 1))
	}
	return strings.Join(events, "\r\n\r\n") + "\r\n\r\n"
}
