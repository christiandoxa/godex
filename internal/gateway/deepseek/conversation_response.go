package deepseek

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func deepSeekTranslatedConversationMessages(body []byte) []any {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil
	}
	messages, _ := value["messages"].([]any)
	return chatcompat.CloneMessages(messages)
}

func deepSeekChatAssistantMessages(body []byte) []any {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return nil
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil
	}
	return chatcompat.ChatAssistantMessages(value)
}

func deepSeekStoreBufferedConversation(
	store chatcompat.ConversationStore,
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
	messages := chatcompat.CloneMessages(conversationMessages)
	messages = append(messages, deepSeekChatAssistantMessages(upstreamBody)...)
	store.Insert(responseID, messages)
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
	store chatcompat.ConversationStore,
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
	messages := chatcompat.CloneMessages(conversationMessages)
	messages = append(messages, deepSeekAnthropicAssistantMessages(upstreamBody)...)
	store.Insert(responseID, messages)
}
