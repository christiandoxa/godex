package kiro

import (
	"sort"
	"strings"
	"sync"
)

const (
	kiroConversationsPerScope = 256
	kiroConversationsTotal    = 2048
	kiroConversationsMaxBytes = 64 << 20
)

type kiroConversation struct {
	history        string
	callIDs        map[string]bool
	sequence       uint64
	estimatedBytes int
}

type kiroConversationStore struct {
	mu             sync.Mutex
	conversations  map[string]kiroConversation
	nextSequence   uint64
	estimatedBytes int
}

func newKiroConversationStore() *kiroConversationStore {
	return &kiroConversationStore{conversations: make(map[string]kiroConversation)}
}

func (store *kiroConversationStore) history(scope, responseID string) (string, bool) {
	if store == nil || strings.TrimSpace(responseID) == "" {
		return "", false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.conversations[kiroConversationKey(scope, responseID)]
	return record.history, ok
}

func (store *kiroConversationStore) findByCallID(scope, callID string) (string, bool) {
	if store == nil || strings.TrimSpace(callID) == "" {
		return "", false
	}
	prefix := kiroConversationScopePrefix(scope)
	store.mu.Lock()
	defer store.mu.Unlock()
	var best kiroConversation
	found := false
	for key, record := range store.conversations {
		if !strings.HasPrefix(key, prefix) || !record.callIDs[callID] {
			continue
		}
		if !found || record.sequence > best.sequence {
			best, found = record, true
		}
	}
	return best.history, found
}

func (store *kiroConversationStore) insert(scope, responseID, history string, callIDs []string) {
	if store == nil || strings.TrimSpace(responseID) == "" || strings.TrimSpace(history) == "" {
		return
	}
	key := kiroConversationKey(scope, responseID)
	ids := make(map[string]bool, len(callIDs))
	for _, callID := range callIDs {
		if callID = strings.TrimSpace(callID); callID != "" {
			ids[callID] = true
		}
	}
	estimated := len(key) + len(history)
	for callID := range ids {
		estimated += len(callID)
	}
	if estimated > kiroConversationsMaxBytes {
		return
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.removeLocked(key)
	store.nextSequence++
	store.conversations[key] = kiroConversation{
		history: history, callIDs: ids, sequence: store.nextSequence, estimatedBytes: estimated,
	}
	store.estimatedBytes += estimated
	store.evictLocked(scope)
}

func (store *kiroConversationStore) removeLocked(key string) {
	if existing, ok := store.conversations[key]; ok {
		store.estimatedBytes -= min(store.estimatedBytes, existing.estimatedBytes)
		delete(store.conversations, key)
	}
}

func (store *kiroConversationStore) evictLocked(scope string) {
	prefix := kiroConversationScopePrefix(scope)
	for store.scopeCountLocked(prefix) > kiroConversationsPerScope {
		if key := store.oldestLocked(prefix); key != "" {
			store.removeLocked(key)
		} else {
			break
		}
	}
	for len(store.conversations) > kiroConversationsTotal || store.estimatedBytes > kiroConversationsMaxBytes {
		if key := store.oldestLocked(""); key != "" {
			store.removeLocked(key)
		} else {
			break
		}
	}
}

func (store *kiroConversationStore) scopeCountLocked(prefix string) int {
	count := 0
	for key := range store.conversations {
		if strings.HasPrefix(key, prefix) {
			count++
		}
	}
	return count
}

func (store *kiroConversationStore) oldestLocked(prefix string) string {
	type candidate struct {
		key      string
		sequence uint64
	}
	items := make([]candidate, 0)
	for key, record := range store.conversations {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			items = append(items, candidate{key: key, sequence: record.sequence})
		}
	}
	if len(items) == 0 {
		return ""
	}
	sort.Slice(items, func(i, j int) bool { return items[i].sequence < items[j].sequence })
	return items[0].key
}

func kiroConversationScopePrefix(scope string) string {
	return "\x00kiro:" + strings.TrimSpace(scope) + ":"
}

func kiroConversationKey(scope, responseID string) string {
	return kiroConversationScopePrefix(scope) + strings.TrimSpace(responseID)
}

func applyKiroConversationHistory(store *kiroConversationStore, scope string, request runtimeRequest) runtimeRequest {
	var history string
	var found bool
	if request.previousResponseID != "" {
		history, found = store.history(scope, request.previousResponseID)
	}
	if !found && request.toolOutputCallID != "" {
		history, found = store.findByCallID(scope, request.toolOutputCallID)
	}
	if found && strings.TrimSpace(history) != "" {
		request.prompt = strings.TrimSpace(history) + "\n\n" + strings.TrimSpace(request.prompt)
	}
	return request
}

func rememberKiroConversation(store *kiroConversationStore, scope string, request runtimeRequest, response map[string]any) {
	if store == nil || responseString(response["status"], "") == "failed" {
		return
	}
	responseID := responseString(response["id"], "")
	if responseID == "" {
		return
	}
	history := strings.TrimSpace(request.prompt)
	if assistant := strings.TrimSpace(kiroResponseText(response)); assistant != "" {
		history += "\n\nAssistant:\n" + assistant
	}
	store.insert(scope, responseID, history, request.callIDs)
}
