package routing

import (
	"errors"
	"strings"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

// rememberVerifiedVolatile retains hard/soft routing affinity during the
// current API-key-pool runtime but does not persist a managed-profile
// ownership binding for a launch-local synthetic account.
func (store *affinityStore) rememberVerifiedVolatile(accountID string, keys affinityKeys, now time.Time) error {
	if strings.TrimSpace(accountID) == "" {
		return errors.New("cannot bind volatile affinity without an account")
	}
	values := keys.values()
	if len(values) == 0 {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)
	store.rememberVerifiedInMemoryLocked(accountID, values, keys, now)
	return nil
}

func (store *affinityStore) rememberVerifiedInMemoryLocked(accountID string, values []string, keys affinityKeys, now time.Time) {
	for _, key := range values {
		current, exists := store.values[key]
		owner := accountID
		if exists && current.accountID != accountID {
			owner = routingentity.ConflictAccountID
		}
		if current.accountID == routingentity.ConflictAccountID {
			owner = routingentity.ConflictAccountID
		}
		store.sequence++
		store.values[key] = affinityValue{accountID: owner, expires: now.Add(affinityTTL), sequence: store.sequence}
	}
	store.touchContinuationEntriesLocked(continuationEntries(keys), now, true)
	store.pruneLocked(now)
}

// registerEphemeralOwner ensures durable bindings from a previous Godex
// version cannot hijack the newly launched transient credential pool.
// The migration is read-only: other managed-profile bindings are untouched.
func (store *affinityStore) registerEphemeralOwner(accountID string) {
	if accountID == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.ephemeralOwners == nil {
		store.ephemeralOwners = make(map[string]struct{})
	}
	if _, exists := store.ephemeralOwners[accountID]; exists {
		return
	}
	store.ephemeralOwners[accountID] = struct{}{}
	for key, binding := range store.values {
		if binding.accountID == accountID && !binding.persistedAt.IsZero() {
			delete(store.values, key)
		}
	}
}
