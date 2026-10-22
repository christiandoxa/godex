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
					switch content.(type) {
					case string, map[string]any, []any:
					default:
						return false
					}
				}
			case "function_call", "custom_tool_call", "mcp_call", "local_shell_call",
				"function_call_output", "custom_tool_call_output", "mcp_tool_result", "mcp_call_output":
			default:
				return false
			}
			continue
		}
		if role, _ := item["role"].(string); role != "" {
			item["type"] = "message"
		}
	}
	return true
}
