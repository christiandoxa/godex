package openai

import (
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	affinityTTL       = 30 * time.Minute
	affinityMaxValues = 4096
	maxAffinityValue  = 4096
)

type affinityKeys struct {
	previous string
	turn     string
	session  string
}

func (keys affinityKeys) values() []string {
	values := make([]string, 0, 3)
	for _, item := range []struct{ prefix, value string }{
		{prefix: "previous:", value: keys.previous},
		{prefix: "turn:", value: keys.turn},
		{prefix: "session:", value: keys.session},
	} {
		prefix, value := item.prefix, item.value
		if value = strings.TrimSpace(value); value != "" && len(value) <= maxAffinityValue {
			values = append(values, prefix+value)
		}
	}
	return values
}

type affinityValue struct {
	accountID string
	expires   time.Time
	sequence  uint64
}

type affinityStore struct {
	mu       sync.Mutex
	values   map[string]affinityValue
	sequence uint64
}

func newAffinityStore() *affinityStore {
	return &affinityStore{values: make(map[string]affinityValue)}
}

func (store *affinityStore) owner(keys affinityKeys, now time.Time) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)

	owner := ""
	for _, key := range keys.values() {
		binding, ok := store.values[key]
		if !ok {
			continue
		}
		if owner != "" && owner != binding.accountID {
			return "", errors.New("request contains conflicting account affinity")
		}
		owner = binding.accountID
	}
	return owner, nil
}

func (store *affinityStore) remember(accountID string, keys affinityKeys, now time.Time) error {
	if strings.TrimSpace(accountID) == "" {
		return errors.New("cannot bind affinity without an account")
	}
	keyValues := keys.values()
	if len(keyValues) == 0 {
		return nil
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)
	for _, key := range keyValues {
		if binding, ok := store.values[key]; ok && binding.accountID != accountID {
			return errors.New("affinity key is already bound to another account")
		}
	}
	for _, key := range keyValues {
		store.sequence++
		store.values[key] = affinityValue{
			accountID: accountID,
			expires:   now.Add(affinityTTL),
			sequence:  store.sequence,
		}
	}
	store.pruneLocked(now)
	return nil
}

func (store *affinityStore) pruneLocked(now time.Time) {
	for key, value := range store.values {
		if !value.expires.After(now) {
			delete(store.values, key)
		}
	}
	// ponytail: bounded O(n) eviction; use a heap only if affinity volume grows.
	for len(store.values) > affinityMaxValues {
		oldestKey := ""
		var oldest uint64
		for key, value := range store.values {
			if oldestKey == "" || value.sequence < oldest {
				oldestKey, oldest = key, value.sequence
			}
		}
		delete(store.values, oldestKey)
	}
}
