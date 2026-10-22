package deepseek

import (
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func deepSeekConversationHistoryForRequest(body []byte, store chatcompat.ConversationStore) []any {
	object, err := parseResponsesObjectWithoutValidation(body)
	if err != nil {
		return nil
	}
	if previous, _ := object["previous_response_id"].(string); strings.TrimSpace(previous) != "" {
		if history := store.History(strings.TrimSpace(previous)); len(history) > 0 {
			return history
		}
	}
	if callID := deepSeekFirstToolOutputCallID(object); callID != "" {
		return store.FindHistoryByCallID(callID)
	}
	return nil
}

func cloneDeepSeekMap(value map[string]any) map[string]any {
	cloned := chatcompat.CloneMessages([]any{value})
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
	result := chatcompat.CloneMessages(messages)
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

func deepSeekFirstToolOutputCallID(object map[string]any) string {
	items, _ := object["input"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		kind, _ := item["type"].(string)
		switch kind {
		case "function_call_output", "custom_tool_call_output", "mcp_tool_result", "mcp_call_output":
			if id := firstStringValue(item, deepSeekCallIDKey, deepSeekToolCallIDKey, "id"); strings.TrimSpace(id) != "" {
				return strings.TrimSpace(id)
			}
		}
	}
	return ""
}
