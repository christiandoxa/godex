package gemini

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
