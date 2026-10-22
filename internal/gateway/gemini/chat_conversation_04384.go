package gemini

import (
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

type geminiChatConversation04384 struct {
	store    chatcompat.ConversationStore
	messages []any
}

func (conversation *geminiChatConversation04384) remember(response, source map[string]any) {
	if conversation == nil || response["status"] == "failed" {
		return
	}
	id, _ := response["id"].(string)
	if strings.TrimSpace(id) == "" {
		return
	}
	messages := append(conversation.messages, geminiChatAssistantMessages04384(source)...)
	conversation.store.Insert(id, messages)
}

func geminiChatAssistantMessages04384(source map[string]any) []any {
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
		cloned := chatcompat.CloneMessages([]any{calls})
		calls = cloned[0]
		items, _ := calls.([]any)
		for _, raw := range items {
			call, _ := raw.(map[string]any)
			function, _ := call["function"].(map[string]any)
			name, _ := function["name"].(string)
			if arguments, ok := function["arguments"].(string); ok {
				function["arguments"] = chatcompat.WrapRTKArguments(name, arguments)
			}
		}
		assistant["tool_calls"] = calls
	}
	return []any{assistant}
}

func geminiChatConversationMessages04384(body []byte) []any {
	request, err := decodeGeminiChatRequest04384(body)
	if err != nil {
		return nil
	}
	messages, _ := request["messages"].([]any)
	return messages
}

func mergeGeminiChatHistory04384(body, input []byte, history []any) ([]byte, error) {
	if len(history) == 0 {
		return body, nil
	}
	request, err := decodeGeminiChatRequest04384(body)
	if err != nil {
		return nil, err
	}
	original, err := decodeGeminiChatRequest04384(input)
	if err != nil {
		return nil, err
	}
	current, _ := request["messages"].([]any)
	var instruction any
	if text, _ := original["instructions"].(string); strings.TrimSpace(text) != "" && len(current) > 0 {
		instruction, current = current[0], current[1:]
		for _, raw := range history {
			message, _ := raw.(map[string]any)
			if message["role"] == "system" && message["content"] == text {
				instruction = nil
				break
			}
		}
	}
	messages := chatcompat.MergeHistory(history, current)
	if instruction != nil {
		messages = append([]any{instruction}, messages...)
	}
	request["messages"] = messages
	return jsonutil.MarshalSerdeValue04384(request)
}
