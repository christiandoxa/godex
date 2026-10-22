package gemini

import "strings"

func normalizeGeminiChatFormat04384(root, controls map[string]any) (map[string]any, bool) {
	format, _, err := geminiResponseFormat(root)
	if err != nil {
		return controls, false
	}
	if format != nil {
		if controls == nil {
			controls = make(map[string]any)
		}
		controls["response_format"] = format
	}
	delete(root, "response_format")
	delete(root, "text")
	return controls, true
}

func ensureGeminiChatJSONInstruction04384(request map[string]any) {
	messages, _ := request["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] != "system" && message["role"] != "user" {
			continue
		}
		if content, ok := message["content"].(string); ok && strings.Contains(geminiASCIILower04384(content), "json") {
			return
		}
	}
	request["messages"] = append([]any{map[string]any{
		"role": "system", "content": "Respond with valid JSON only.",
	}}, messages...)
}

func geminiChatFormatMetadata04384(root, metadata map[string]any) {
	kind := responseFormatType(root)
	if kind != "json_schema" && kind != "structured_output" {
		return
	}
	provider, _ := metadata["gemini"].(map[string]any)
	if provider == nil {
		provider = make(map[string]any)
		metadata["gemini"] = provider
	}
	provider["degraded_response_format"] = map[string]any{
		"from": kind, "to": "json_object",
		"reason": "Gemini OpenAI-compatible response_format supports json_object but not native JSON Schema enforcement",
	}
}
