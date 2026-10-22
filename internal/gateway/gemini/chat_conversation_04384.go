package gemini

import (
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type geminiChatConversation04384 struct {
	store    chatcompat.ConversationStore
	messages []any
}

func (transport *RuntimeTransport) conversationsForRequest(input proxymodel.Request) chatcompat.ConversationStore {
	if transport.scopeConversations {
		return transport.conversations.ForRequest(input)
	}
	return transport.conversations
}

func (conversation *geminiChatConversation04384) remember(response, source map[string]any) {
	if conversation == nil || response["status"] == "failed" {
		return
	}
	id, _ := response["id"].(string)
	if strings.TrimSpace(id) == "" {
		return
	}
	messages := append(conversation.messages, chatcompat.ChatAssistantMessages(source)...)
	conversation.store.Insert(id, messages)
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
