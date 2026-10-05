package deepseek

import (
	"encoding/json"
	"strings"
)

func deepSeekTranslatedConversationMessages(body []byte) []any {
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return nil
	}
	messages, _ := value["messages"].([]any)
	return cloneDeepSeekMessages(messages)
}

func deepSeekChatAssistantMessages(body []byte) []any {
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return nil
	}
	choices, _ := value["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	if message == nil {
		return nil
	}
	result := cloneDeepSeekMap(message)
	result["role"] = "assistant"
	if calls, _ := result["tool_calls"].([]any); len(calls) > 0 {
		if content, ok := result[deepSeekContentKey]; !ok || content == nil {
			result[deepSeekContentKey] = ""
		}
	}
	return []any{result}
}

func deepSeekStoreBufferedConversation(
	store deepSeekConversationStore,
	conversationMessages []any,
	upstreamBody, translatedBody []byte,
) {
	var translated map[string]any
	if json.Unmarshal(translatedBody, &translated) != nil || translated["status"] == "failed" {
		return
	}
	responseID, _ := translated["id"].(string)
	if strings.TrimSpace(responseID) == "" {
		return
	}
	messages := cloneDeepSeekMessages(conversationMessages)
	messages = append(messages, deepSeekChatAssistantMessages(upstreamBody)...)
	store.insert(responseID, messages)
}

func deepSeekAnthropicAssistantMessages(body []byte) []any {
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return nil
	}
	content, _ := value["content"].([]any)
	if len(content) == 0 {
		return nil
	}
	var text strings.Builder
	toolCalls := make([]any, 0)
	for _, raw := range content {
		block, _ := raw.(map[string]any)
		if block == nil {
			continue
		}
		switch block["type"] {
		case "text":
			if value, _ := block["text"].(string); value != "" {
				text.WriteString(value)
			}
		case "tool_use":
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
				continue
			}
			input := block["input"]
			if input == nil {
				input = map[string]any{}
			}
			arguments, err := json.Marshal(input)
			if err != nil {
				continue
			}
			toolCalls = append(toolCalls, map[string]any{
				"id":       id,
				"type":     "function",
				"function": map[string]any{"name": name, "arguments": string(arguments)},
			})
		}
	}
	if text.Len() == 0 && len(toolCalls) == 0 {
		return nil
	}
	return []any{map[string]any{"role": "assistant", "content": text.String(), "tool_calls": toolCalls}}
}

func deepSeekStoreAnthropicConversation(
	store deepSeekConversationStore,
	conversationMessages []any,
	upstreamBody, translatedBody []byte,
) {
	var translated map[string]any
	if json.Unmarshal(translatedBody, &translated) != nil || translated["status"] == "failed" {
		return
	}
	responseID, _ := translated["id"].(string)
	if strings.TrimSpace(responseID) == "" {
		return
	}
	messages := cloneDeepSeekMessages(conversationMessages)
	messages = append(messages, deepSeekAnthropicAssistantMessages(upstreamBody)...)
	store.insert(responseID, messages)
}
