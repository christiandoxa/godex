package routing

import (
	"context"
	"errors"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (router *Router) resolveRequestOwner(ctx context.Context, request proxymodel.Request, accounts []proxymodel.Account, keys affinityKeys) (string, func() error, error) {
	rawKeys := len(accounts) != 0
	for _, account := range accounts {
		if !account.EphemeralAPIKey {
			rawKeys = false
			break
		}
	}
	if !rawKeys {
		if err := router.repairConflictedFullHistorySession043711(ctx, request, keys); err != nil {
			return "", nil, err
		}
		return router.resolveOwner(ctx, keys, request.QuotaSelection)
	}
	// Local-rewrite dispatch checks response, turn and session lineage before
	// credential selection; an unresolved bound identity never becomes fresh work.
	keys.thread = ""
	owner, err := router.affinity.owner(ctx, keys, router.now())
	if err != nil && !errors.Is(err, errConflictingAccountAffinity) {
		return "", nil, err
	}
	if owner != "" || err != nil {
		return "", nil, &proxymodel.Error{StatusCode: http.StatusServiceUnavailable,
			Message: "bound continuation provider is unavailable"}
	}
	return "", nil, nil
}
