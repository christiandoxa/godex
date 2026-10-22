package gemini

import "strings"

func normalizeGeminiChatInputItems04384(value any) bool {
	entries, _ := value.([]any)
	for _, raw := range entries {
		item, ok := raw.(map[string]any)
		if _, hasPrefix := item["prefix"]; !ok || hasPrefix {
			return false
		}
		if kind, hasKind := item["type"]; hasKind {
			switch kind {
			case "message":
				if role, present := item["role"]; present {
					switch role {
					case "user", "assistant", "system", "developer":
					default:
						return false
					}
				}
				if content, present := item["content"]; present {
					if !validGeminiChatContent04384(content) {
						return false
					}
				}
			case "function_call", "custom_tool_call", "mcp_call", "local_shell_call",
				"function_call_output", "custom_tool_call_output", "mcp_tool_result", "mcp_call_output":
				if !validGeminiChatCallIdentity04384(item) || !validGeminiChatCallName04384(kind, item) ||
					!validGeminiChatToolOutput04384(kind, item) {
					return false
				}
				if !projectGeminiChatFunctionItem04384(kind, item) {
					return false
				}
				continue
			default:
				return false
			}
		}
		if !projectGeminiChatMessage04384(item) {
			return false
		}
	}
	return true
}

func validGeminiChatCallIdentity04384(item map[string]any) bool {
	for _, field := range []string{"call_id", "tool_call_id", "id"} {
		if identity, ok := item[field].(string); ok {
			return strings.TrimSpace(identity) != ""
		}
	}
	return false
}

func validGeminiChatCallName04384(kind any, item map[string]any) bool {
	switch kind {
	case "function_call", "custom_tool_call", "mcp_call":
	default:
		return true
	}
	// The tagged scanner probes its literal sentinel before nested function.name.
	for _, field := range []string{"name", "tool_name", "__missing__"} {
		if name, ok := item[field].(string); ok {
			return strings.TrimSpace(name) != ""
		}
	}
	function, _ := item["function"].(map[string]any)
	name, _ := function["name"].(string)
	return strings.TrimSpace(name) != ""
}

func validGeminiChatToolOutput04384(kind any, item map[string]any) bool {
	switch kind {
	case "function_call_output", "custom_tool_call_output", "mcp_tool_result", "mcp_call_output":
	default:
		return true
	}
	for _, field := range []string{"output", "content", "result", "error"} {
		if _, present := item[field]; present {
			return true
		}
	}
	return false
}

func projectGeminiChatMessage04384(item map[string]any) bool {
	content, present := item["content"]
	if !present {
		content, present = item["text"]
	}
	text := ""
	if present {
		var err error
		text, err = geminiChatContentText04384(content)
		if err != nil {
			return false
		}
	}
	role, _ := item["role"].(string)
	switch role {
	case "assistant", "tool", "system":
	case "developer":
		role = "system"
	default:
		role = "user"
	}
	item["type"], item["role"], item["content"] = "message", role, text
	return true
}
