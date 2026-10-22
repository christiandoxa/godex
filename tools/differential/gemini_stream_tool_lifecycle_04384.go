package main

import (
	"encoding/json"
	"io"
	"slices"
)

type geminiLifecycleTool04384 struct {
	index               uint64
	id, name, arguments string
	fragments           []string
}

func geminiStreamToolLifecycleCases04384() []string {
	return []string{"stream-chat-tool-simple", "stream-chat-tool-fragments", "stream-chat-tool-multiple", "stream-chat-tool-mixed-text", "stream-chat-tool-invalid-arguments", "stream-chat-tool-missing-name", "stream-chat-tool-shell", "stream-chat-tool-search", "stream-chat-tool-patch", "stream-chat-tool-empty-arguments"}
}

func geminiStreamToolLifecycleCase04384(scenario string) bool {
	return slices.Contains(geminiStreamToolLifecycleCases04384(), scenario)
}

func geminiLifecycleTools04384(scenario string) []geminiLifecycleTool04384 {
	tool := geminiLifecycleTool04384{index: 2, id: "call_synthetic", name: "lookup", arguments: `{}`}
	switch scenario {
	case "stream-chat-tool-fragments":
		tool.arguments, tool.fragments = `{"x":1}`, []string{`{"x":`, `1}`}
	case "stream-chat-tool-multiple":
		return []geminiLifecycleTool04384{{index: 8, id: "call_synthetic_8", name: "lookup", arguments: `{}`}, tool}
	case "stream-chat-tool-invalid-arguments":
		tool.arguments = `{bad`
	case "stream-chat-tool-missing-name":
		tool.name = ""
	case "stream-chat-tool-shell":
		tool.name, tool.arguments = "shell", `{"cmd":"git status"}`
	case "stream-chat-tool-search":
		tool.name, tool.arguments = "tool_search", `{"q":"synthetic"}`
	case "stream-chat-tool-patch":
		tool.name, tool.arguments = "apply_patch", `{"input":"synthetic patch"}`
	case "stream-chat-tool-empty-arguments":
		tool.arguments = ""
	}
	return []geminiLifecycleTool04384{tool}
}

func writeGeminiToolLifecycle04384(writer io.Writer, scenario string) {
	var calls []any
	tools := geminiLifecycleTools04384(scenario)
	for _, tool := range tools {
		arguments := tool.arguments
		if len(tool.fragments) != 0 {
			arguments = tool.fragments[0]
		}
		function := map[string]any{"arguments": arguments}
		if tool.name != "" {
			function["name"] = tool.name
		}
		calls = append(calls, map[string]any{"index": tool.index, "id": tool.id, "function": function})
	}
	delta := map[string]any{"tool_calls": calls}
	if scenario == "stream-chat-tool-mixed-text" {
		delta["content"] = "synthetic-"
	}
	packet := map[string]any{"created": 1, "model": "gemini-3.1-pro-preview", "choices": []any{map[string]any{"delta": delta}}}
	writeGeminiLifecyclePacket04384(writer, packet)
	for _, tool := range tools {
		if len(tool.fragments) < 2 {
			continue
		}
		for _, fragment := range tool.fragments[1:] {
			writeGeminiLifecyclePacket04384(writer, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": tool.index, "function": map[string]any{"arguments": fragment}}}}}}})
		}
	}
	finish := "tool_calls"
	if scenario == "stream-chat-tool-mixed-text" {
		finish = "stop"
		writeGeminiLifecyclePacket04384(writer, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "ok<>&/🌍\u2028\u2029"}}}})
	}
	writeGeminiLifecyclePacket04384(writer, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": finish}}})
	writeGeminiLifecyclePacket04384(writer, map[string]any{"choices": []any{}, "usage": map[string]any{"prompt_tokens": uint64(9007199254740993), "completion_tokens": 1, "total_tokens": uint64(9007199254740994)}})
	_, _ = io.WriteString(writer, "data: [DONE]\n\n")
}

func writeGeminiLifecyclePacket04384(writer io.Writer, packet map[string]any) {
	encoded, _ := json.Marshal(packet)
	_, _ = io.WriteString(writer, "data: "+string(encoded)+"\n\n")
}
