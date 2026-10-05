package deepseek

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	deepSeekConversationScopePrefix      = "\x00prodex:"
	deepSeekConversationsPerScope        = 256
	deepSeekConversationsTotal           = 2048
	deepSeekConversationsMaxEstimateByte = 64 << 20
	deepSeekConversationNamespaceHeader  = "x-prodex-internal-conversation-namespace"
)

type deepSeekConversation struct {
	messages       []any
	sequence       uint64
	estimatedBytes int
}

type deepSeekConversationState struct {
	mu            sync.Mutex
	conversations map[string]deepSeekConversation
	nextSequence  uint64
	estimated     int
}

type deepSeekConversationStore struct {
	state       *deepSeekConversationState
	scopePrefix string
}

func newDeepSeekConversationStore() deepSeekConversationStore {
	return deepSeekConversationStore{state: &deepSeekConversationState{conversations: make(map[string]deepSeekConversation)}}
}

func (store deepSeekConversationStore) scoped(namespace string) deepSeekConversationStore {
	return deepSeekConversationStore{
		state:       store.state,
		scopePrefix: fmt.Sprintf("%s%d:%s:", deepSeekConversationScopePrefix, len(namespace), namespace),
	}
}

func (store deepSeekConversationStore) storageKey(responseID string) string {
	if store.scopePrefix == "" {
		return responseID
	}
	return store.scopePrefix + responseID
}

func (store deepSeekConversationStore) keyInScope(key string) bool {
	if store.scopePrefix == "" {
		return !strings.HasPrefix(key, deepSeekConversationScopePrefix)
	}
	return strings.HasPrefix(key, store.scopePrefix)
}

func (store deepSeekConversationStore) history(responseID string) []any {
	if store.state == nil || strings.TrimSpace(responseID) == "" {
		return nil
	}
	store.state.mu.Lock()
	defer store.state.mu.Unlock()
	conversation, ok := store.state.conversations[store.storageKey(responseID)]
	if !ok {
		return nil
	}
	return cloneDeepSeekMessages(conversation.messages)
}

func (store deepSeekConversationStore) contains(responseID string) bool {
	if store.state == nil || strings.TrimSpace(responseID) == "" {
		return false
	}
	store.state.mu.Lock()
	defer store.state.mu.Unlock()
	_, ok := store.state.conversations[store.storageKey(responseID)]
	return ok
}

func (store deepSeekConversationStore) findHistoryByCallID(callID string) []any {
	if store.state == nil || strings.TrimSpace(callID) == "" {
		return nil
	}
	store.state.mu.Lock()
	defer store.state.mu.Unlock()
	var selected deepSeekConversation
	found := false
	for key, conversation := range store.state.conversations {
		if !store.keyInScope(key) || !deepSeekHistoryHasToolCall(conversation.messages, callID) {
			continue
		}
		if !found || conversation.sequence > selected.sequence {
			selected, found = conversation, true
		}
	}
	if !found {
		return nil
	}
	return cloneDeepSeekMessages(selected.messages)
}

func (store deepSeekConversationStore) insert(responseID string, messages []any) {
	responseID = strings.TrimSpace(responseID)
	if store.state == nil || responseID == "" {
		return
	}
	messages = cloneDeepSeekMessages(messages)
	key := store.storageKey(responseID)
	estimated := deepSeekConversationEstimate(key, messages)
	if estimated > deepSeekConversationsMaxEstimateByte {
		return
	}

	store.state.mu.Lock()
	defer store.state.mu.Unlock()
	if previous, ok := store.state.conversations[key]; ok {
		store.state.estimated -= previous.estimatedBytes
		delete(store.state.conversations, key)
	}
	store.state.nextSequence++
	store.state.conversations[key] = deepSeekConversation{
		messages: messages, sequence: store.state.nextSequence, estimatedBytes: estimated,
	}
	store.state.estimated += estimated

	for store.scopeCountLocked() > deepSeekConversationsPerScope {
		key := store.oldestLocked(true)
		if key == "" {
			break
		}
		store.removeLocked(key)
	}
	for len(store.state.conversations) > deepSeekConversationsTotal || store.state.estimated > deepSeekConversationsMaxEstimateByte {
		key := store.oldestLocked(false)
		if key == "" {
			break
		}
		store.removeLocked(key)
	}
}

func (store deepSeekConversationStore) scopeCountLocked() int {
	count := 0
	for key := range store.state.conversations {
		if store.keyInScope(key) {
			count++
		}
	}
	return count
}

func (store deepSeekConversationStore) oldestLocked(scopeOnly bool) string {
	key := ""
	var sequence uint64
	for candidate, conversation := range store.state.conversations {
		if scopeOnly && !store.keyInScope(candidate) {
			continue
		}
		if key == "" || conversation.sequence < sequence {
			key, sequence = candidate, conversation.sequence
		}
	}
	return key
}

func (store deepSeekConversationStore) removeLocked(key string) {
	if conversation, ok := store.state.conversations[key]; ok {
		store.state.estimated -= conversation.estimatedBytes
		delete(store.state.conversations, key)
	}
}

func deepSeekConversationEstimate(key string, messages []any) int {
	size := len(key)
	for _, message := range messages {
		size = deepSeekEstimateAdd(size, deepSeekJSONEstimatedBytes(message))
		if size > deepSeekConversationsMaxEstimateByte {
			return deepSeekConversationsMaxEstimateByte + 1
		}
	}
	return size
}

func deepSeekJSONEstimatedBytes(value any) int {
	const (
		jsonValueNodeBytes = 32
		jsonNumberBytes    = 16
		jsonBoolBytes      = 1
	)
	payload := 0
	switch typed := value.(type) {
	case nil:
	case bool:
		payload = jsonBoolBytes
	case json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		payload = jsonNumberBytes
	case string:
		payload = len(typed)
	case []any:
		for _, item := range typed {
			payload = deepSeekEstimateAdd(payload, deepSeekJSONEstimatedBytes(item))
		}
	case map[string]any:
		for key, item := range typed {
			payload = deepSeekEstimateAdd(payload, len(key))
			payload = deepSeekEstimateAdd(payload, deepSeekJSONEstimatedBytes(item))
		}
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return deepSeekConversationsMaxEstimateByte + 1
		}
		var normalized any
		decoder := json.NewDecoder(strings.NewReader(string(encoded)))
		decoder.UseNumber()
		if decoder.Decode(&normalized) != nil {
			return deepSeekConversationsMaxEstimateByte + 1
		}
		return deepSeekJSONEstimatedBytes(normalized)
	}
	return deepSeekEstimateAdd(jsonValueNodeBytes, payload)
}

func deepSeekEstimateAdd(left, right int) int {
	if left > deepSeekConversationsMaxEstimateByte || right > deepSeekConversationsMaxEstimateByte-left {
		return deepSeekConversationsMaxEstimateByte + 1
	}
	return left + right
}

func cloneDeepSeekMessages(messages []any) []any {
	if len(messages) == 0 {
		return nil
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		return nil
	}
	var cloned []any
	if json.Unmarshal(encoded, &cloned) != nil {
		return nil
	}
	return cloned
}

func deepSeekHistoryHasToolCall(messages []any, callID string) bool {
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		calls, _ := message["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			if id, _ := call["id"].(string); id == callID {
				return true
			}
		}
	}
	return false
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

func (transport *RuntimeTransport) conversationsForRequest(request proxymodel.Request) deepSeekConversationStore {
	namespace := deepSeekRequestSessionID(request)
	if namespace == "" {
		namespace = deepSeekHeaderValue(request.Header, deepSeekConversationNamespaceHeader)
	}
	if namespace == "" {
		namespace = "gateway"
	}
	return transport.conversations.scoped(namespace)
}

func deepSeekRequestSessionID(request proxymodel.Request) string {
	for _, name := range []string{"session_id", "session-id", "x-session-id"} {
		if value := deepSeekHeaderValue(request.Header, name); value != "" {
			return value
		}
	}
	if metadata := deepSeekHeaderValue(request.Header, "x-codex-turn-metadata"); metadata != "" {
		if value := deepSeekSessionIDFromJSON([]byte(metadata)); value != "" {
			return value
		}
	}
	return deepSeekSessionIDFromJSON(request.Body)
}

func deepSeekHeaderValue(header map[string][]string, wanted string) string {
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

func deepSeekSessionIDFromJSON(body []byte) string {
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
