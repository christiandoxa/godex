package chatcompat

// ChatAssistantMessages projects validated Chat JSON into isolated replay messages.
func ChatAssistantMessages(source map[string]any) []any {
	choices, _ := source["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	content, contentIsString := message["content"].(string)
	reasoning, _ := message["reasoning_content"].(string)
	calls, hasCalls := message["tool_calls"]
	if content == "" && reasoning == "" && !hasCalls {
		return nil
	}
	assistant := map[string]any{"role": "assistant", "content": nil}
	if contentIsString {
		assistant["content"] = content
	} else if hasCalls {
		assistant["content"] = ""
	}
	if reasoning != "" {
		assistant["reasoning_content"] = reasoning
	}
	if hasCalls {
		cloned := CloneMessages([]any{calls})
		calls = cloned[0]
		items, _ := calls.([]any)
		for _, raw := range items {
			call, _ := raw.(map[string]any)
			function, _ := call["function"].(map[string]any)
			name, _ := function["name"].(string)
			if arguments, ok := function["arguments"].(string); ok {
				function["arguments"] = WrapRTKArguments(name, arguments)
			}
		}
		assistant["tool_calls"] = calls
	}
	return []any{assistant}
}
