package chatcompat

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

const (
	conversationScopePrefix      = "\x00prodex:"
	conversationsPerScope        = 256
	conversationsTotal           = 2048
	conversationsMaxEstimateByte = 64 << 20
)

type conversation struct {
	messages       []any
	sequence       uint64
	estimatedBytes int
}

type conversationState struct {
	mu            sync.Mutex
	conversations map[string]conversation
	nextSequence  uint64
	estimated     int
}

// ConversationStore owns bounded, scoped Chat protocol snapshots; its zero value is inert.
type ConversationStore struct {
	state       *conversationState
	scopePrefix string
}

// NewConversationStore allocates a cache owned by one runtime transport.
func NewConversationStore() ConversationStore {
	return ConversationStore{state: &conversationState{conversations: make(map[string]conversation)}}
}

func (store ConversationStore) Scoped(namespace string) ConversationStore {
	return ConversationStore{
		state:       store.state,
		scopePrefix: fmt.Sprintf("%s%d:%s:", conversationScopePrefix, len(namespace), namespace),
	}
}

func (store ConversationStore) storageKey(responseID string) string {
	if store.scopePrefix == "" {
		return responseID
	}
	return store.scopePrefix + responseID
}

func (store ConversationStore) keyInScope(key string) bool {
	if store.scopePrefix == "" {
		return !strings.HasPrefix(key, conversationScopePrefix)
	}
	return strings.HasPrefix(key, store.scopePrefix)
}

func (store ConversationStore) History(responseID string) []any {
	if store.state == nil || strings.TrimSpace(responseID) == "" {
		return nil
	}
	store.state.mu.Lock()
	defer store.state.mu.Unlock()
	conversation, ok := store.state.conversations[store.storageKey(responseID)]
	if !ok {
		return nil
	}
	return CloneMessages(conversation.messages)
}

func (store ConversationStore) Contains(responseID string) bool {
	if store.state == nil || strings.TrimSpace(responseID) == "" {
		return false
	}
	store.state.mu.Lock()
	defer store.state.mu.Unlock()
	_, ok := store.state.conversations[store.storageKey(responseID)]
	return ok
}

func (store ConversationStore) FindHistoryByCallID(callID string) []any {
	if store.state == nil || strings.TrimSpace(callID) == "" {
		return nil
	}
	store.state.mu.Lock()
	defer store.state.mu.Unlock()
	var selected conversation
	found := false
	for key, conversation := range store.state.conversations {
		if !store.keyInScope(key) || !historyHasToolCall(conversation.messages, callID) {
			continue
		}
		if !found || conversation.sequence > selected.sequence {
			selected, found = conversation, true
		}
	}
	if !found {
		return nil
	}
	return CloneMessages(selected.messages)
}

func (store ConversationStore) Insert(responseID string, messages []any) {
	responseID = strings.TrimSpace(responseID)
	if store.state == nil || responseID == "" {
		return
	}
	messages = CloneMessages(messages)
	key := store.storageKey(responseID)
	estimated := conversationEstimate(key, messages)
	if estimated > conversationsMaxEstimateByte {
		return
	}

	store.state.mu.Lock()
	defer store.state.mu.Unlock()
	if previous, ok := store.state.conversations[key]; ok {
		store.state.estimated -= previous.estimatedBytes
		delete(store.state.conversations, key)
	}
	store.state.nextSequence++
	store.state.conversations[key] = conversation{
		messages: messages, sequence: store.state.nextSequence, estimatedBytes: estimated,
	}
	store.state.estimated += estimated

	for store.scopeCountLocked() > conversationsPerScope {
		key := store.oldestLocked(true)
		if key == "" {
			break
		}
		store.removeLocked(key)
	}
	for len(store.state.conversations) > conversationsTotal || store.state.estimated > conversationsMaxEstimateByte {
		key := store.oldestLocked(false)
		if key == "" {
			break
		}
		store.removeLocked(key)
	}
}

func (store ConversationStore) scopeCountLocked() int {
	count := 0
	for key := range store.state.conversations {
		if store.keyInScope(key) {
			count++
		}
	}
	return count
}

func (store ConversationStore) oldestLocked(scopeOnly bool) string {
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

func (store ConversationStore) removeLocked(key string) {
	if conversation, ok := store.state.conversations[key]; ok {
		store.state.estimated -= conversation.estimatedBytes
		delete(store.state.conversations, key)
	}
}

func conversationEstimate(key string, messages []any) int {
	size := len(key)
	for _, message := range messages {
		size = estimateAdd(size, jsonEstimatedBytes(message))
		if size > conversationsMaxEstimateByte {
			return conversationsMaxEstimateByte + 1
		}
	}
	return size
}

func jsonEstimatedBytes(value any) int {
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
			payload = estimateAdd(payload, jsonEstimatedBytes(item))
		}
	case map[string]any:
		for key, item := range typed {
			payload = estimateAdd(payload, len(key))
			payload = estimateAdd(payload, jsonEstimatedBytes(item))
		}
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return conversationsMaxEstimateByte + 1
		}
		var normalized any
		decoder := json.NewDecoder(strings.NewReader(string(encoded)))
		decoder.UseNumber()
		if decoder.Decode(&normalized) != nil {
			return conversationsMaxEstimateByte + 1
		}
		return jsonEstimatedBytes(normalized)
	}
	return estimateAdd(jsonValueNodeBytes, payload)
}

func estimateAdd(left, right int) int {
	if left > conversationsMaxEstimateByte || right > conversationsMaxEstimateByte-left {
		return conversationsMaxEstimateByte + 1
	}
	return left + right
}

// CloneMessages makes an independent JSON snapshot of Chat messages.
func CloneMessages(messages []any) []any {
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

func historyHasToolCall(messages []any, callID string) bool {
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
