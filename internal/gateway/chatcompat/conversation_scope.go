package chatcompat

import (
	"encoding/json"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const conversationNamespaceHeader = "x-prodex-internal-conversation-namespace"

// ForRequest selects the protocol namespace without changing account ownership.
func (store ConversationStore) ForRequest(request proxymodel.Request) ConversationStore {
	namespace := requestSessionID(request)
	if namespace == "" {
		namespace = conversationHeaderValue(request.Header, conversationNamespaceHeader)
	}
	if namespace == "" {
		namespace = "gateway"
	}
	return store.Scoped(namespace)
}

func requestSessionID(request proxymodel.Request) string {
	for _, name := range []string{"session_id", "session-id", "x-session-id"} {
		if value := conversationHeaderValue(request.Header, name); value != "" {
			return value
		}
	}
	if metadata := conversationHeaderValue(request.Header, "x-codex-turn-metadata"); metadata != "" {
		if value := sessionIDFromJSON([]byte(metadata)); value != "" {
			return value
		}
	}
	return sessionIDFromJSON(request.Body)
}

func conversationHeaderValue(header map[string][]string, wanted string) string {
	for name, values := range header {
		if !strings.EqualFold(name, wanted) {
			continue
		}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func sessionIDFromJSON(body []byte) string {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return ""
	}
	if value, _ := object["session_id"].(string); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	metadata, _ := object["client_metadata"].(map[string]any)
	if value, _ := metadata["session_id"].(string); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}
