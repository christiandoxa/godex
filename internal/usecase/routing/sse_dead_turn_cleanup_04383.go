package routing

import (
	"context"
	"strings"
	"time"
)

// releaseDeadStreamOrphanTurn04383 mirrors the tagged
// clear_runtime_dead_turn_state_bindings planner: a dead response's turn
// owner is removed only if *no surviving response on the same account*
// still references that token. Durable profiles require inspecting the
// persisted response->turn sidecars, rather than trusting only the bounded
// in-memory response-turn cache (which may be incomplete after a restart).
func (store *affinityStore) releaseDeadStreamOrphanTurn04383(
	ctx context.Context, accountID, profileHome, turn string, now time.Time,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	turn = strings.TrimSpace(turn)
	if accountID == "" || !validHardBindingComponent04383(turn) {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)
	if store.repository != nil {
		// If a durable source cannot establish all surviving relationships,
		// fail closed by *retaining* the turn instead of removing one still
		// used by a concurrent or cold-start continuation.
		stateRepo, ok := store.repository.(turnStateRepository)
		if !ok || profileHome == "" {
			return nil
		}
		bindings, err := store.repository.Load(ctx)
		if err != nil {
			return err
		}
		for _, binding := range bindings {
			if binding.Kind != "previous" || binding.AccountID != accountID {
				continue
			}
			value, _, err := stateRepo.LoadResponseTurnState(ctx, profileHome, binding.Key, now)
			if err != nil {
				return err
			}
			if value == turn {
				return nil
			}
		}
	} else {
		for key, value := range store.turnStates {
			if value.accountID != accountID || value.value != turn || !value.expires.After(now) {
				continue
			}
			if binding, ok := store.values[key]; ok && binding.accountID == accountID && binding.expires.After(now) {
				return nil
			}
		}
	}
	turnKeys := affinityKeys{turn: turn}
	if err := store.loadMissingLocked(ctx, turnKeys, now); err != nil {
		return err
	}
	key := affinityDigest("turn", turn)
	binding, ok := store.values[key]
	if !ok || binding.accountID != accountID {
		return nil
	}
	if status, found := store.statuses[key]; found {
		store.markContinuationDeadLocked(status.kind, key, now)
		if err := store.persistContinuationDeadLocked(ctx, status.kind, key, now); err != nil {
			return err
		}
	}
	if store.repository != nil && store.writesEnabled() {
		if err := store.repository.Remove(ctx, []string{key}); err != nil {
			return err
		}
	}
	delete(store.values, key)
	return nil
}
