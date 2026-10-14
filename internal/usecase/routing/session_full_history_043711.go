package routing

import (
	"context"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// Tagged Prodex 0.437.11 permits a precommit repair of a *conflicted soft
// session only* when the full history is reconstructable. Opaque previous
// response IDs, turn-state tokens and hard compact/session bindings never
// take this path, nor can committed output be retroactively replayed.
func (router *Router) repairConflictedFullHistorySession043711(
	ctx context.Context, request proxymodel.Request, keys affinityKeys,
) error {
	if request.QuotaSelection.RouteKind != quotamodel.RouteKindResponses ||
		!keys.hasSoftSessionAffinity(request.QuotaSelection) ||
		!requestHasReconstructableFullHistory(request) {
		return nil
	}
	sessionKey := affinityDigest("session", keys.session)
	store := router.affinity
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(router.now())
	// The release decision is based on the latest committed durable binding,
	// not an old cached conflict in the local process.
	if store.repository != nil {
		bindings, err := store.repository.Load(ctx)
		if err != nil {
			return err
		}
		delete(store.values, sessionKey)
		store.loadLocked(bindings, []string{sessionKey}, router.now())
	}
	current, ok := store.values[sessionKey]
	if !ok || current.accountID != routingentity.ConflictAccountID {
		return nil
	}
	if store.repository != nil && store.writesEnabled() {
		if err := store.repository.Remove(ctx, []string{sessionKey}); err != nil {
			return err
		}
	}
	delete(store.values, sessionKey)
	store.removeContinuationStatusLocked(sessionKey)
	return nil
}
