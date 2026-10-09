package routing

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (store *affinityStore) forget(ctx context.Context, keys affinityKeys) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	values := keys.values()
	if len(values) == 0 {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.repository != nil && store.writesEnabled() {
		if err := store.repository.Remove(ctx, values); err != nil {
			return fmt.Errorf("remove conversation affinity: %w", err)
		}
	}
	for _, key := range values {
		delete(store.values, key)
		store.removeContinuationStatusLocked(key)
	}
	return nil
}

func (store *affinityStore) forgetDeadResponse(
	ctx context.Context,
	responseID, accountID, profileHome string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	responseID = strings.TrimSpace(responseID)
	if responseID == "" || accountID == "" || len(responseID) > maxAffinityValue {
		return nil
	}
	key := affinityDigest("previous", responseID)
	store.mu.Lock()
	defer store.mu.Unlock()
	if binding, ok := store.values[key]; ok && binding.accountID != accountID {
		return nil
	}
	now := time.Now()
	if store.clock != nil {
		now = store.clock()
	}
	store.markContinuationDeadLocked("response", key, now)
	if err := store.persistContinuationDeadLocked(ctx, "response", key, now); err != nil {
		return err
	}
	if repository, ok := store.repository.(responseTurnStateRemover); ok && profileHome != "" && store.writesEnabled() {
		if err := repository.RemoveResponseTurnState(ctx, profileHome, key); err != nil {
			return fmt.Errorf("remove stale response turn state: %w", err)
		}
	}
	if store.repository != nil && store.writesEnabled() {
		if err := store.repository.Remove(ctx, []string{key}); err != nil {
			return fmt.Errorf("remove stale response binding: %w", err)
		}
	}
	delete(store.values, key)
	if state, ok := store.turnStates[key]; ok && state.accountID == accountID {
		delete(store.turnStates, key)
	}
	return nil
}

func (store *affinityStore) releasePreviousResponse(
	ctx context.Context,
	responseID, accountID, profileHome string,
	keys affinityKeys,
) error {
	if err := store.forgetDeadResponse(ctx, responseID, accountID, profileHome); err != nil {
		return err
	}
	return store.forgetAccount(ctx, accountID, affinityKeys{turn: keys.turn, session: keys.session})
}

func (store *affinityStore) forgetAccount(ctx context.Context, accountID string, keys affinityKeys) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	values := make([]string, 0, len(keys.values()))
	for _, key := range keys.values() {
		if value, ok := store.values[key]; ok && value.accountID == accountID {
			values = append(values, key)
		}
	}
	now := time.Now()
	if store.clock != nil {
		now = store.clock()
	}
	for _, key := range values {
		if status, ok := store.statuses[key]; ok {
			store.markContinuationDeadLocked(status.kind, key, now)
			if err := store.persistContinuationDeadLocked(ctx, status.kind, key, now); err != nil {
				return err
			}
		}
	}
	if store.repository != nil && len(values) > 0 && store.writesEnabled() {
		if err := store.repository.Remove(ctx, values); err != nil {
			return fmt.Errorf("remove account affinity: %w", err)
		}
	}
	for _, key := range values {
		delete(store.values, key)
	}
	return nil
}
