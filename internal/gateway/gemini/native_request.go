package gemini

import "strings"

func geminiGenerateContentRequest(chat, original map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	contents := make([]any, 0)
	var systemParts []any
	toolNames := make(map[string]string)

	messages, _ := chat["messages"].([]any)
	for index := 0; index < len(messages); index++ {
		raw := messages[index]
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		switch role {
		case "system", "developer":
			for _, part := range geminiMessageParts(message["content"]) {
				if text, ok := part.(map[string]any)["text"].(string); ok {
					systemParts = append(systemParts, map[string]any{"text": text})
				}
			}
		case "assistant":
			parts := geminiMessageParts(message["content"])
			calls, _ := message["tool_calls"].([]any)
			for _, rawCall := range calls {
				call, _ := rawCall.(map[string]any)
				function, _ := call["function"].(map[string]any)
				name, _ := function["name"].(string)
				if strings.TrimSpace(name) == "" {
					name = "tool_call"
				}
				args := geminiJSONArguments(function["arguments"])
				item := map[string]any{"name": name, "args": args}
				if callID, _ := call["id"].(string); callID != "" {
					toolNames[callID] = name
					item["id"] = callID
				}
				if extra, ok := call["extra_content"].(map[string]any); ok {
					if google, ok := extra["google"].(map[string]any); ok {
						if sig, _ := google["thought_signature"].(string); sig != "" {
							item["thoughtSignature"] = sig
						}
					}
				}
				parts = append(parts, map[string]any{"functionCall": item})
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "model", "parts": parts})
			}
		case "tool":
			parts := make([]any, 0)
			for index < len(messages) {
				next, _ := messages[index].(map[string]any)
				if next == nil {
					break
				}
				nextRole, _ := next["role"].(string)
				if nextRole != "tool" {
					break
				}
				callID, _ := next["tool_call_id"].(string)
				name, _ := next["name"].(string)
				if name == "" {
					name = toolNames[callID]
				}
				if name == "" {
					name = callID
				}
				item := map[string]any{"name": name, "response": geminiToolResponseValue(next["content"])}
				if callID != "" {
					item["id"] = callID
				}
				parts = append(parts, map[string]any{"functionResponse": item})
				index++
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "user", "parts": parts})
			}
			index--
			continue
		default:
			parts := geminiMessageParts(message["content"])
			if len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "user", "parts": parts})
			}
		}
	}

	contextual := make([]string, 0)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message == nil {
			continue
		}
		role, _ := message["role"].(string)
		if role != "user" {
			continue
		}
		for _, part := range geminiMessageParts(message["content"]) {
			if text, ok := part.(map[string]any)["text"].(string); ok && geminiIsContextualInstruction(text) {
				contextual = append(contextual, text)
			}
		}
	}
	if len(contextual) > 0 {
		for _, text := range contextual {
			for index := 0; index < len(contents); index++ {
				content, _ := contents[index].(map[string]any)
				parts, _ := content["parts"].([]any)
				if len(parts) == 1 {
					if value, _ := parts[0].(map[string]any)["text"].(string); value == text {
						contents = append(contents[:index], contents[index+1:]...)
						index--
						break
					}
				}
			}
		}
		if len(systemParts) == 0 {
			systemParts = make([]any, 0, len(contextual))
		}
		for _, text := range contextual {
			systemParts = append(systemParts, map[string]any{"text": text})
		}
	}
	if len(systemParts) > 0 {
		result["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": geminiJoinSystemParts(systemParts)}}}
	}
	if len(contents) > 0 {
		result["contents"] = contents
	}

	tools := geminiNativeBuiltinTools(original["tools"])
	tools = append(tools, geminiNativeTools(chat)...)
	if len(tools) > 0 {
		result["tools"] = tools
	}
	if config := geminiNativeToolConfig(chat["tool_choice"]); config != nil {
		result["toolConfig"] = config
	}
	generation, err := geminiNativeGenerationConfig(original, chat)
	if err != nil {
		return nil, err
	}
	if len(generation) > 0 {
		result["generationConfig"] = generation
	}
	for _, pair := range [][2]string{{"safety_settings", "safetySettings"}, {"safetySettings", "safetySettings"}, {"cached_content", "cachedContent"}, {"cachedContent", "cachedContent"}, {"labels", "labels"}} {
		if value, ok := original[pair[0]]; ok {
			if _, exists := result[pair[1]]; !exists && value != nil {
				result[pair[1]] = value
			}
		}
	}
	return result, nil
}
