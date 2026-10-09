package routing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"slices"
	"strings"
	"sync"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
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

func (keys affinityKeys) hasAffinity() bool {
	return keys.previous != "" || keys.turn != "" || keys.session != "" || keys.thread != ""
}

func (keys affinityKeys) hasHardAffinity(selection quotamodel.Selection) bool {
	return keys.previous != "" || keys.turn != "" || keys.thread != "" ||
		(selection.RouteKind == quotamodel.RouteKindCompact && keys.session != "")
}

func (keys affinityKeys) hasSoftSessionAffinity(selection quotamodel.Selection) bool {
	return keys.session != "" && keys.previous == "" && keys.turn == "" && keys.thread == "" &&
		selection.RouteKind != quotamodel.RouteKindCompact
}

func (keys affinityKeys) entries() []routingentity.Binding {
	values := make([]routingentity.Binding, 0, 4)
	for _, item := range []struct{ kind, value string }{{"previous", keys.previous}, {"turn", keys.turn}, {"session", keys.session}, {"thread", keys.thread}} {
		value := strings.TrimSpace(item.value)
		if value == "" || len(value) > maxAffinityValue {
			continue
		}
		values = append(values, routingentity.Binding{Key: affinityDigest(item.kind, value), Kind: item.kind})
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
	Remove(context.Context, []string) error
	AcquireConversation(context.Context) (func() error, error)
}

type verifiedBindingRepository interface {
	MergeVerified(context.Context, []routingentity.Binding) ([]routingentity.Binding, error)
}

type affinityStore struct {
	repository         bindingRepository
	mu                 sync.Mutex
	values             map[string]affinityValue
	turnStates         map[string]responseTurnState
	statuses           map[string]continuationStatus
	clock              func() time.Time
	persistenceEnabled func() bool
	ephemeralOwners    map[string]struct{}
	sequence           uint64
}

func newAffinityStore() *affinityStore {
	return &affinityStore{
		values:   make(map[string]affinityValue),
		statuses: make(map[string]continuationStatus),
		clock:    time.Now,
	}
}

func (store *affinityStore) writesEnabled() bool {
	return store.persistenceEnabled == nil || store.persistenceEnabled()
}

func (store *affinityStore) owner(ctx context.Context, keys affinityKeys, now time.Time) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)
	if err := store.loadMissingLocked(ctx, keys, now); err != nil {
		return "", err
	}
	owner, err := store.ownerLocked(keys)
	if err == nil && owner != "" {
		entries := make([]bindingEntry, 0, len(keys.entries()))
		for _, entry := range continuationEntries(keys) {
			if _, ok := store.values[entry.Key]; ok {
				entries = append(entries, entry)
			}
		}
		store.touchContinuationEntriesLocked(entries, now, false)
	}
	return owner, err
}

func (store *affinityStore) refreshOwner(ctx context.Context, keys affinityKeys, now time.Time) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)
	if store.repository == nil {
		return store.ownerLocked(keys)
	}
	bindings, err := store.repository.Load(ctx)
	if err != nil {
		return "", err
	}
	keyValues := keys.values()
	for _, key := range keyValues {
		delete(store.values, key)
	}
	store.loadLocked(bindings, keyValues, now)
	owner, err := store.ownerLocked(keys)
	if err == nil && owner != "" {
		entries := make([]bindingEntry, 0, len(keys.entries()))
		for _, entry := range continuationEntries(keys) {
			if _, ok := store.values[entry.Key]; ok {
				entries = append(entries, entry)
			}
		}
		store.touchContinuationEntriesLocked(entries, now, false)
	}
	return owner, err
}

func (store *affinityStore) loadMissingLocked(ctx context.Context, keys affinityKeys, now time.Time) error {
	if store.repository == nil {
		return nil
	}
	keyValues := keys.values()
	missing := false
	for _, key := range keyValues {
		if _, ok := store.values[key]; !ok {
			missing = true
			break
		}
	}
	if !missing {
		return nil
	}
	bindings, err := store.repository.Load(ctx)
	if err != nil {
		return err
	}
	store.loadLocked(bindings, keyValues, now)
	return nil
}

func (store *affinityStore) ownerLocked(keys affinityKeys) (string, error) {
	owner := ""
	for _, key := range keys.values() {
		binding, ok := store.values[key]
		if !ok {
			continue
		}
		if binding.accountID == routingentity.ConflictAccountID {
			return "", errors.New("request contains conflicting account affinity")
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
	if err := store.checkConflictsLocked(accountID, keyValues); err != nil {
		return err
	}
	if err := store.persistLocked(ctx, accountID, keys, now); err != nil {
		return err
	}
	store.refreshLocked(accountID, keyValues, now)
	store.touchContinuationEntriesLocked(continuationEntries(keys), now, false)
	store.pruneLocked(now)
	return nil
}

func (store *affinityStore) rememberVerified(ctx context.Context, accountID string, keys affinityKeys, now time.Time) error {
	if strings.TrimSpace(accountID) == "" {
		return errors.New("cannot bind verified affinity without an account")
	}
	keyValues := keys.values()
	if len(keyValues) == 0 {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)
	if err := store.loadMissingLocked(ctx, keys, now); err != nil {
		return err
	}
	if repository, ok := store.repository.(verifiedBindingRepository); ok && store.writesEnabled() {
		updates := make([]routingentity.Binding, 0, len(keys.entries()))
		for _, entry := range keys.entries() {
			updates = append(updates, routingentity.Binding{
				Key: entry.Key, Kind: entry.Kind, AccountID: accountID, UpdatedUnix: now.Unix(),
			})
		}
		bindings, err := repository.MergeVerified(ctx, updates)
		if err != nil {
			return err
		}
		store.loadLocked(bindings, keyValues, now)
		store.touchContinuationEntriesLocked(continuationEntries(keys), now, true)
		return nil
	}
	store.rememberVerifiedInMemoryLocked(accountID, keyValues, keys, now)
	return nil
}

func (store *affinityStore) checkConflictsLocked(accountID string, keys []string) error {
	for _, key := range keys {
		if binding, ok := store.values[key]; ok && binding.accountID != accountID {
			return errors.New("affinity key is already bound to another account")
		}
	}
	return nil
}

func (store *affinityStore) persistLocked(ctx context.Context, accountID string, keys affinityKeys, now time.Time) error {
	if store.repository == nil || !store.writesEnabled() {
		return nil
	}
	updates := make([]routingentity.Binding, 0, len(keys.values()))
	for _, entry := range keys.entries() {
		old, ok := store.values[entry.Key]
		if ok && !old.persistedAt.Before(now.Add(-24*time.Hour)) {
			continue
		}
		updates = append(updates, routingentity.Binding{
			Key: entry.Key, Kind: entry.Kind, AccountID: accountID, UpdatedUnix: now.Unix(),
		})
	}
	if len(updates) == 0 {
		return nil
	}
	bindings, err := store.repository.Merge(ctx, updates)
	if err != nil {
		return err
	}
	store.loadLocked(bindings, keys.values(), now)
	return nil
}

func (store *affinityStore) refreshLocked(accountID string, keys []string, now time.Time) {
	for _, key := range keys {
		persistedAt := store.values[key].persistedAt
		if store.repository == nil {
			persistedAt = time.Time{}
		}
		store.sequence++
		store.values[key] = affinityValue{
			accountID: accountID, expires: now.Add(affinityTTL),
			sequence: store.sequence, persistedAt: persistedAt,
		}
	}
}

func (store *affinityStore) pruneLocked(now time.Time) {
	for key, value := range store.values {
		if !value.expires.After(now) {
			delete(store.values, key)
		}
	}
	for key, value := range store.turnStates {
		if !value.expires.After(now) {
			delete(store.turnStates, key)
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
	for len(store.turnStates) > affinityMaxValues {
		oldestKey := ""
		var oldest uint64
		for key, value := range store.turnStates {
			if oldestKey == "" || value.sequence < oldest {
				oldestKey, oldest = key, value.sequence
			}
		}
		delete(store.turnStates, oldestKey)
	}
}

func (store *affinityStore) loadLocked(bindings []routingentity.Binding, keys []string, now time.Time) {
	for i := len(bindings) - 1; i >= 0; i-- {
		binding := bindings[i]
		if !slices.Contains(keys, binding.Key) {
			continue
		}
		if _, ephemeral := store.ephemeralOwners[binding.AccountID]; ephemeral {
			// An earlier Godex version may have saved a launch-local
			// API-key affinity. Treat it as stale, not a managed owner.
			continue
		}
		store.sequence++
		store.values[binding.Key] = affinityValue{accountID: binding.AccountID, expires: now.Add(affinityTTL), sequence: store.sequence, persistedAt: time.Unix(binding.UpdatedUnix, 0)}
	}
	store.pruneLocked(now)
}

func affinityDigest(kind, value string) string {
	digest := sha256.Sum256([]byte(kind + ":" + value))
	return hex.EncodeToString(digest[:])
}
