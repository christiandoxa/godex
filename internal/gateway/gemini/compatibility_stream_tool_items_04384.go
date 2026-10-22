package gemini

import (
	"encoding/json"
	"strings"
)

func geminiChatStreamSplitToolName04384(name string) (string, string) {
	if index := strings.LastIndex(name, "--"); index >= 0 {
		namespace, short := name[:index], name[index+2:]
		if strings.TrimSpace(namespace) != "" && strings.TrimSpace(short) != "" {
			return namespace, short
		}
	}
	if rest, present := strings.CutPrefix(name, "mcp__"); present {
		if index := strings.LastIndex(rest, "__"); index >= 0 && strings.TrimSpace(rest[index+2:]) != "" {
			return "mcp__" + rest[:index], rest[index+2:]
		}
	}
	return "", name
}

func geminiChatAddedToolItem04384(id, name string) map[string]any {
	if name == "tool_search" {
		return nil
	}
	if name == "apply_patch" {
		return map[string]any{"type": "custom_tool_call", "call_id": id, "name": name, "input": ""}
	}
	namespace, short := geminiChatStreamSplitToolName04384(name)
	item := map[string]any{"type": "function_call", "call_id": id, "name": short}
	if namespace != "" {
		item["namespace"] = namespace
	}
	return item
}

func geminiChatCompletedToolItem04384(tool *geminiChatStreamTool04384) map[string]any {
	arguments := tool.arguments.String()
	if tool.name == "tool_search" || tool.name == "apply_patch" {
		var value any
		decoder := json.NewDecoder(strings.NewReader(arguments))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			if tool.name == "apply_patch" {
				return map[string]any{"type": "custom_tool_call", "call_id": tool.id, "name": tool.name, "input": arguments}
			}
			value = map[string]any{}
		}
		return geminiNativeToolCallItem(nil, map[string]any{"id": tool.id, "name": tool.name, "args": value}, tool.id)
	}
	item := geminiChatAddedToolItem04384(tool.id, tool.name)
	item["arguments"] = arguments
	if tool.signature != "" {
		item["gemini_thought_signature"] = tool.signature
	}
	return item
}
