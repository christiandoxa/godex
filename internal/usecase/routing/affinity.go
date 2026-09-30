package routing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
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
	thread   string
}

func (keys affinityKeys) entries() []routingentity.Binding {
	values := make([]routingentity.Binding, 0, 4)
	for _, item := range []struct{ kind, value string }{{"previous", keys.previous}, {"turn", keys.turn}, {"session", keys.session}, {"thread", keys.thread}} {
		value := strings.TrimSpace(item.value)
		if value == "" || len(value) > maxAffinityValue {
			continue
		}
		digest := sha256.Sum256([]byte(item.kind + ":" + value))
		values = append(values, routingentity.Binding{Key: hex.EncodeToString(digest[:]), Kind: item.kind})
	}
	return values
}
func (keys affinityKeys) values() []string {
	entries := keys.entries()
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Key)
	}
	return values
}

type affinityValue struct {
	accountID   string
	expires     time.Time
	sequence    uint64
	persistedAt time.Time
}

type bindingRepository interface {
	Load(context.Context) ([]routingentity.Binding, error)
	Merge(context.Context, []routingentity.Binding) ([]routingentity.Binding, error)
}

type affinityStore struct {
	repository bindingRepository
	mu         sync.Mutex
	values     map[string]affinityValue
	sequence   uint64
}

func newAffinityStore() *affinityStore {
	return &affinityStore{values: make(map[string]affinityValue)}
}

func (store *affinityStore) owner(ctx context.Context, keys affinityKeys, now time.Time) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)

	if store.repository != nil {
		missing := false
		for _, key := range keys.values() {
			if _, ok := store.values[key]; !ok {
				missing = true
			}
		}
		if missing {
			bindings, err := store.repository.Load(ctx)
			if err != nil {
				return "", err
			}
			store.loadLocked(bindings, now)
		}
	}
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

func (store *affinityStore) remember(ctx context.Context, accountID string, keys affinityKeys, now time.Time) error {
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
	if store.repository != nil {
		updates := make([]routingentity.Binding, 0, len(keyValues))
		for _, entry := range keys.entries() {
			key := entry.Key
			if old, ok := store.values[key]; !ok || old.persistedAt.Before(now.Add(-24*time.Hour)) {
				updates = append(updates, routingentity.Binding{Key: key, Kind: entry.Kind, AccountID: accountID, UpdatedUnix: now.Unix()})
			}
		}
		if len(updates) > 0 {
			bindings, err := store.repository.Merge(ctx, updates)
			if err != nil {
				return err
			}
			store.loadLocked(bindings, now)
		}
	}
	for _, key := range keyValues {
		persistedAt := store.values[key].persistedAt
		if store.repository == nil {
			persistedAt = time.Time{}
		}
		store.sequence++
		store.values[key] = affinityValue{
			accountID:   accountID,
			expires:     now.Add(affinityTTL),
			sequence:    store.sequence,
			persistedAt: persistedAt,
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

func (store *affinityStore) loadLocked(bindings []routingentity.Binding, now time.Time) {
	for i := len(bindings) - 1; i >= 0; i-- {
		binding := bindings[i]
		store.sequence++
		store.values[binding.Key] = affinityValue{accountID: binding.AccountID, expires: now.Add(affinityTTL), sequence: store.sequence, persistedAt: time.Unix(binding.UpdatedUnix, 0)}
	}
	store.pruneLocked(now)
}
