package routing

import (
	"context"
	"fmt"
)

// The broker's native root-session release can arrive after a prior worker
// removed its binding. Persist the release fence for both ordinary and compact
// session aliases in one repository transaction, even when neither exists.
func (store *affinityStore) forgetSessionGroup043711(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	keys := []string{
		affinityDigest("session", sessionID),
		affinityDigest("session", "__compact_session__:"+sessionID),
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.repository != nil && store.writesEnabled() {
		if releaser, ok := store.repository.(interface {
			ReleaseSessions(context.Context, []string) error
		}); ok {
			if err := releaser.ReleaseSessions(ctx, keys); err != nil {
				return fmt.Errorf("release durable session affinity: %w", err)
			}
		} else if err := store.repository.Remove(ctx, keys); err != nil {
			return fmt.Errorf("remove session affinity: %w", err)
		}
	}
	for _, key := range keys {
		delete(store.values, key)
		store.removeContinuationStatusLocked(key)
	}
	return nil
}
