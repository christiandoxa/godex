package gemini

func restoreGeminiChatCallSignatures04384(request map[string]any, input []byte) error {
	root, err := decodeGeminiChatRequest04384(input)
	if err != nil {
		return err
	}
	signatures := geminiChatInputSignatures04384(root["input"])
	messages, _ := request["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		calls, _ := message["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			id, _ := call["id"].(string)
			signature := signatures[id]
			if signature == "" {
				continue
			}
			if geminiSignatureHasMojoNonSpace043711(signature) {
				call["extra_content"] = map[string]any{"google": map[string]any{"thought_signature": signature}}
			} else {
				// The later preservation kernel leaves the selected blank legacy field intact.
				call["gemini_thought_signature"] = signature
			}
		}
	}
	return nil
}

func geminiChatInputSignatures04384(value any) map[string]string {
	entries, _ := value.([]any)
	signatures := make(map[string]string)
	for _, raw := range entries {
		item, _ := raw.(map[string]any)
		switch item["type"] {
		case "function_call", "custom_tool_call", "mcp_call", "local_shell_call":
		default:
			continue
		}
		id := geminiChatWireCallIdentity04384(item)
		if _, seen := signatures[id]; seen {
			continue
		}
		// Adjacency keeps the first call for an ID, including an unsigned call.
		signatures[id] = ""
		if item["type"] != "local_shell_call" {
			signatures[id] = geminiChatInputSignature04384(item)
		}
	}
	return signatures
}

func geminiChatInputSignature04384(item map[string]any) string {
	provider, _ := item["provider_specific_fields"].(map[string]any)
	extra, _ := item["extra_content"].(map[string]any)
	google, _ := extra["google"].(map[string]any)
	for _, value := range []any{item["gemini_thought_signature"], item["thought_signature"], item["thoughtSignature"],
		provider["thought_signature"], google["thought_signature"]} {
		if signature, ok := value.(string); ok && signature != "" {
			return signature
		}
	}
	return ""
}
