package deepseek

import "strings"

func deepSeekConversationHistoryForRequest(body []byte, store deepSeekConversationStore) []any {
	object, err := parseResponsesObjectWithoutValidation(body)
	if err != nil {
		return nil
	}
	if previous, _ := object["previous_response_id"].(string); strings.TrimSpace(previous) != "" {
		if history := store.history(strings.TrimSpace(previous)); len(history) > 0 {
			return history
		}
	}
	if callID := deepSeekFirstToolOutputCallID(object); callID != "" {
		return store.findHistoryByCallID(callID)
	}
	return nil
}

func mergeDeepSeekHistory(history, current []any) []any {
	if len(history) == 0 {
		return current
	}
	result := cloneDeepSeekMessages(history)
	toolCalls := deepSeekToolCallIDs(history)
	toolOutputs := deepSeekToolOutputIDs(history)
	signatures := deepSeekMessageSignatures(history)
	for _, raw := range current {
		message, _ := raw.(map[string]any)
		if message == nil {
			continue
		}
		role, _ := message["role"].(string)
		content, contentIsString := message[deepSeekContentKey].(string)
		if contentIsString && strings.TrimSpace(content) != "" {
			if signatures[role+"\x00"+content] {
				continue
			}
		}
		if role == "tool" {
			callID, _ := message[deepSeekToolCallIDKey].(string)
			if callID != "" && toolOutputs[callID] {
				continue
			}
		}
		if role == "assistant" {
			calls, _ := message["tool_calls"].([]any)
			if len(calls) > 0 {
				filtered := make([]any, 0, len(calls))
				for _, rawCall := range calls {
					call, _ := rawCall.(map[string]any)
					id, _ := call["id"].(string)
					if id != "" && toolCalls[id] {
						continue
					}
					filtered = append(filtered, rawCall)
				}
				if len(filtered) == 0 && strings.TrimSpace(content) == "" {
					continue
				}
				copy := cloneDeepSeekMap(message)
				copy["tool_calls"] = filtered
				raw = copy
			}
		}
		result = append(result, raw)
	}
	return result
}

func deepSeekToolCallIDs(messages []any) map[string]bool {
	result := make(map[string]bool)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		calls, _ := message["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			if id, _ := call["id"].(string); strings.TrimSpace(id) != "" {
				result[id] = true
			}
		}
	}
	return result
}

func deepSeekToolOutputIDs(messages []any) map[string]bool {
	result := make(map[string]bool)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] != "tool" {
			continue
		}
		if id, _ := message[deepSeekToolCallIDKey].(string); strings.TrimSpace(id) != "" {
			result[id] = true
		}
	}
	return result
}

func deepSeekMessageSignatures(messages []any) map[string]bool {
	result := make(map[string]bool)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		content, _ := message[deepSeekContentKey].(string)
		if strings.TrimSpace(content) != "" {
			result[role+"\x00"+content] = true
		}
	}
	return result
}

func cloneDeepSeekMap(value map[string]any) map[string]any {
	cloned := cloneDeepSeekMessages([]any{value})
	if len(cloned) != 1 {
		return map[string]any{}
	}
	result, _ := cloned[0].(map[string]any)
	return result
}

func repairDeepSeekToolCallAdjacency(messages []any) []any {
	outputs := make(map[string]map[string]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message == nil || message["role"] != "tool" {
			continue
		}
		id, _ := message[deepSeekToolCallIDKey].(string)
		id = strings.TrimSpace(id)
		if id != "" {
			if _, exists := outputs[id]; !exists {
				outputs[id] = message
			}
		}
	}
	emitted := make(map[string]bool)
	result := make([]any, 0, len(messages))
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message == nil {
			continue
		}
		if message["role"] == "tool" && len(outputs) > 0 {
			continue
		}
		calls, _ := message["tool_calls"].([]any)
		if len(calls) == 0 {
			result = append(result, message)
			continue
		}
		selected := make([]any, 0, len(calls))
		selectedIDs := make([]string, 0, len(calls))
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			id, _ := call["id"].(string)
			id = strings.TrimSpace(id)
			if id == "" || emitted[id] || outputs[id] == nil {
				continue
			}
			emitted[id] = true
			selected = append(selected, rawCall)
			selectedIDs = append(selectedIDs, id)
		}
		if len(selected) == 0 {
			if !deepSeekMessageHasContent(message) {
				continue
			}
			copy := cloneDeepSeekMap(message)
			delete(copy, "tool_calls")
			result = append(result, copy)
			continue
		}
		copy := cloneDeepSeekMap(message)
		copy["tool_calls"] = selected
		if content, exists := copy[deepSeekContentKey]; !exists || content == nil {
			copy[deepSeekContentKey] = ""
		}
		result = append(result, copy)
		for _, id := range selectedIDs {
			result = append(result, cloneDeepSeekMap(outputs[id]))
		}
	}
	return result
}

func deepSeekMessageHasContent(message map[string]any) bool {
	return deepSeekValueHasContent(message[deepSeekContentKey]) || deepSeekValueHasContent(message["reasoning_content"])
}

func deepSeekValueHasContent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func normalizeDeepSeekThinkingToolCallMessages(messages []any) []any {
	result := cloneDeepSeekMessages(messages)
	for index, raw := range result {
		message, _ := raw.(map[string]any)
		if message == nil || message["role"] != "assistant" {
			continue
		}
		calls, _ := message["tool_calls"].([]any)
		if len(calls) == 0 {
			continue
		}
		copy := cloneDeepSeekMap(message)
		if content, exists := copy[deepSeekContentKey]; !exists || content == nil {
			copy[deepSeekContentKey] = ""
		}
		if _, ok := copy["reasoning_content"].(string); !ok {
			copy["reasoning_content"] = ""
		}
		result[index] = copy
	}
	return result
}
