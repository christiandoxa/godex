package gemini

const geminiChatThinkingChoiceReason04384 = "Gemini OpenAI-compatible thinking mode currently rejects explicit tool_choice on the OpenAI Chat route, so Prodex omits it while preserving translated function tools"

func normalizeGeminiChatReasoning04384(root, controls map[string]any) (map[string]any, bool) {
	if value, present := root["reasoning"]; present {
		reasoning, ok := value.(map[string]any)
		if !ok {
			return controls, false
		}
		for key := range reasoning {
			if key != "effort" {
				return controls, false
			}
		}
	}
	effort, err := geminiReasoningEffort(root)
	if err != nil {
		return controls, false
	}
	if effort != "" {
		if effort == "xhigh" || effort == "max" {
			effort = "high"
		}
		if controls == nil {
			controls = make(map[string]any)
		}
		controls["reasoning_effort"] = effort
	}
	if geminiChatThinking04384(controls) {
		delete(root, "tool_choice")
	}
	delete(root, "reasoning")
	delete(root, "reasoning_effort")
	return controls, true
}

func geminiChatThinking04384(controls map[string]any) bool {
	switch controls["reasoning_effort"] {
	case "high", "medium", "low":
		return true
	default:
		return false
	}
}

func geminiChatReasoningMetadata04384(body []byte, controls map[string]any) (map[string]any, error) {
	if !geminiChatThinking04384(controls) {
		return nil, nil
	}
	root, err := decodeGeminiChatRequest04384(body)
	if err != nil {
		return nil, err
	}
	choice, present := root["tool_choice"]
	if !present {
		return nil, nil
	}
	return map[string]any{"gemini": map[string]any{"omitted_tool_choice": map[string]any{
		"from": choice, "reason": geminiChatThinkingChoiceReason04384,
	}}}, nil
}

func normalizeGeminiChatThinkingMessages04384(request map[string]any) {
	messages, _ := request["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		calls, _ := message["tool_calls"].([]any)
		if message["role"] != "assistant" || len(calls) == 0 {
			continue
		}
		if message["content"] == nil {
			message["content"] = ""
		}
		if _, ok := message["reasoning_content"].(string); !ok {
			message["reasoning_content"] = ""
		}
	}
}
