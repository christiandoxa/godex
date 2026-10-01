package routing

import (
	"context"
	"errors"
)

type conversationLock struct {
	ready chan struct{}
	users int
}

// Serialize one conversation through commitment; unrelated threads remain parallel.
func (router *Router) acquireConversation(ctx context.Context, keys affinityKeys) (func(), error) {
	keyValues := affinityKeys{thread: keys.thread, session: keys.session}.values()
	if len(keyValues) == 0 {
		return noConversationRelease, nil
	}
	key := keyValues[len(keyValues)-1]
	router.mu.Lock()
	if router.conversations == nil {
		router.conversations = make(map[string]*conversationLock)
	}
	slot := router.conversations[key]
	if slot == nil {
		if len(router.conversations) >= affinityMaxValues {
			router.mu.Unlock()
			return nil, errors.New("too many active conversations")
		}
		slot = &conversationLock{ready: make(chan struct{}, 1)}
		router.conversations[key] = slot
	}
	slot.users++
	router.mu.Unlock()
	drop := func() {
		router.mu.Lock()
		slot.users--
		if slot.users == 0 {
			delete(router.conversations, key)
		}
		router.mu.Unlock()
	}
	select {
	case slot.ready <- struct{}{}:
		return func() { <-slot.ready; drop() }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

func noConversationRelease() {
	// No stable conversation key means no local conversation lock was acquired.
}
