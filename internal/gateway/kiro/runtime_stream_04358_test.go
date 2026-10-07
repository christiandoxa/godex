package kiro

import (
	"strings"
	"testing"
)

func TestProdex04358MessagesSSEKeepsEventOrderUnicodeAndToolUse(t *testing.T) {
	message := map[string]any{
		"id":    "resp_界",
		"type":  "message",
		"role":  "assistant",
		"model": "kiro",
		"content": []any{
			map[string]any{"type": "text", "text": "hello 🐈"},
			map[string]any{"type": "text", "text": ""},
			map[string]any{"type": "tool_use", "id": "call_1", "name": "run", "input": map[string]any{"z": 1, "a": "é"}},
			map[string]any{"type": "unknown", "value": true},
		},
		"stop_reason":   "tool_use",
		"stop_sequence": nil,
		"usage":         map[string]any{"input_tokens": 3, "output_tokens": 5},
	}
	got := string(kiroMessagesSSE(message))
	want := strings.Join([]string{
		"event: message_start",
		"data: {\"message\":{\"content\":[],\"id\":\"resp_界\",\"model\":\"kiro\",\"role\":\"assistant\",\"stop_reason\":null,\"stop_sequence\":null,\"type\":\"message\",\"usage\":{\"input_tokens\":3,\"output_tokens\":5}},\"type\":\"message_start\"}",
		"",
		"event: content_block_start",
		"data: {\"content_block\":{\"text\":\"\",\"type\":\"text\"},\"index\":0,\"type\":\"content_block_start\"}",
		"",
		"event: content_block_delta",
		"data: {\"delta\":{\"text\":\"hello 🐈\",\"type\":\"text_delta\"},\"index\":0,\"type\":\"content_block_delta\"}",
		"",
		"event: content_block_stop",
		"data: {\"index\":0,\"type\":\"content_block_stop\"}",
		"",
		"event: content_block_start",
		"data: {\"content_block\":{\"text\":\"\",\"type\":\"text\"},\"index\":1,\"type\":\"content_block_start\"}",
		"",
		"event: content_block_stop",
		"data: {\"index\":1,\"type\":\"content_block_stop\"}",
		"",
		"event: content_block_start",
		"data: {\"content_block\":{\"id\":\"call_1\",\"input\":{},\"name\":\"run\",\"type\":\"tool_use\"},\"index\":2,\"type\":\"content_block_start\"}",
		"",
		"event: content_block_delta",
		"data: {\"delta\":{\"partial_json\":\"{\\\"a\\\":\\\"é\\\",\\\"z\\\":1}\",\"type\":\"input_json_delta\"},\"index\":2,\"type\":\"content_block_delta\"}",
		"",
		"event: content_block_stop",
		"data: {\"index\":2,\"type\":\"content_block_stop\"}",
		"",
		"event: message_delta",
		"data: {\"delta\":{\"stop_reason\":\"tool_use\",\"stop_sequence\":null},\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5}}",
		"",
		"event: message_stop",
		"data: {\"type\":\"message_stop\"}",
		"",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("Anthropic SSE mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestProdex04358MessagesResponsePreservesToolUseShape(t *testing.T) {
	response := map[string]any{
		"id": map[string]any{"synthetic": "response"},
		"output": []any{
			map[string]any{"type": "function_call", "call_id": 17, "name": false, "arguments": "[1,\"é\"]"},
			map[string]any{"type": "message", "content": []any{map[string]any{"text": "done 🐈"}}},
			map[string]any{"type": "function_call", "arguments": "not json"},
		},
		"usage":    map[string]any{"input_tokens": "3", "output_tokens": nil},
		"metadata": map[string]any{"kiro": map[string]any{"stop_reason": "max_output_tokens"}},
	}
	got := kiroMessagesResponse(response, "kiro-模型")
	content := got["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("content = %#v", content)
	}
	first := content[0].(map[string]any)
	if first["type"] != "tool_use" || first["id"] != 17 || first["name"] != false {
		t.Fatalf("first tool use = %#v", first)
	}
	input := first["input"].([]any)
	if len(input) != 2 || input[0] != float64(1) || input[1] != "é" {
		t.Fatalf("first tool input = %#v", input)
	}
	second := content[1].(map[string]any)
	if second["id"] != "call_kiro" || second["name"] != "tool_call" {
		t.Fatalf("default tool use = %#v", second)
	}
	if got["stop_reason"] != "tool_use" {
		t.Fatalf("stop_reason = %#v", got["stop_reason"])
	}
	usage := got["usage"].(map[string]any)
	if usage["input_tokens"] != "3" || usage["output_tokens"] != nil {
		t.Fatalf("usage = %#v", usage)
	}
}
