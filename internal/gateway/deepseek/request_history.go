package deepseek

import (
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/jsonutil"
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
		if strings.TrimSpace(id) != "" {
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
			if strings.TrimSpace(id) == "" || emitted[id] || outputs[id] == nil {
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
				return id
			}
		}
	}
	return ""
}

func translateResponsesRequestWithHistory(body []byte, options RequestOptions, history []any, contains func(string) bool) (TranslatedRequest, error) {
	object, err := parseResponsesObject(body)
	if err != nil {
		return TranslatedRequest{}, err
	}
	projection := normalMessages
	if contains == nil {
		projection = standaloneMessages
	}
	parts, err := translateRequestParts(object, options, projection)
	if err != nil {
		return TranslatedRequest{}, err
	}
	stateless := parts
	override := contains != nil && deepSeekSimpleRequest04384(object, contains)
	if override {
		stateless.tools, _ = object["tools"].([]any)
		stateless.messages, err = deepSeekMessages(object, simpleMessages)
		if err != nil {
			return TranslatedRequest{}, err
		}
	}
	parts.messages = repairDeepSeekToolCallAdjacency(chatcompat.MergeHistory(history, parts.messages))
	if contains != nil && len(parts.messages) == 0 {
		parts.messages = []any{map[string]any{"role": "user", deepSeekContentKey: ""}}
	}
	if parts.thinking {
		parts.messages = normalizeDeepSeekThinkingToolCallMessages(parts.messages)
	}
	metadata, err := deepSeekResponseMetadata(object, parts.thinking)
	if err != nil {
		return TranslatedRequest{}, err
	}
	result, err := buildTranslatedRequest(object, options.Model, parts)
	if err != nil {
		return TranslatedRequest{}, err
	}
	content, err := jsonutil.MarshalSerdeValue04384(result)
	if err != nil {
		return TranslatedRequest{}, err
	}
	var pending []any
	if contains != nil {
		pending = deepSeekTranslatedConversationMessages(content)
		if override {
			if stateless.thinking {
				stateless.messages = normalizeDeepSeekThinkingToolCallMessages(stateless.messages)
			}
			if stateless.messages == nil {
				stateless.messages = []any{}
			}
			result, err = buildTranslatedRequest(object, options.Model, stateless)
			if err != nil {
				return TranslatedRequest{}, err
			}
			content, err = jsonutil.MarshalSerdeValue04384(result)
			if err != nil {
				return TranslatedRequest{}, err
			}
		}
	}
	return TranslatedRequest{Body: content, ResponseMetadata: metadata, conversationMessages: pending}, nil
}
