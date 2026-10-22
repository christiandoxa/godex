package routing

import (
	"context"
	"errors"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) resolveRequestOwner(ctx context.Context, request proxymodel.Request, accounts []proxymodel.Account, keys affinityKeys) (string, func() error, error) {
	deepSeek := rawKeyPool(accounts, "deepseek")
	gemini := rawKeyPool(accounts, "gemini")
	if (deepSeek || gemini) && !rawKeyLocalRoute(request) {
		for _, value := range []string{keys.previous, keys.turn, keys.session} {
			if strings.TrimSpace(value) != "" && !validHardBindingComponent04383(value) {
				return "", nil, &proxymodel.Error{StatusCode: http.StatusServiceUnavailable,
					Message: "bound continuation provider is unavailable"}
			}
		}
	}
	if !deepSeek && (!gemini || rawKeyLocalRoute(request)) {
		if err := router.repairConflictedFullHistorySession043711(ctx, request, keys); err != nil {
			return "", nil, err
		}
		return router.resolveOwner(ctx, keys, request.QuotaSelection)
	}
	if rawKeyLocalRoute(request) {
		return "", nil, nil
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

func rawKeyPool(accounts []proxymodel.Account, provider string) bool {
	if len(accounts) == 0 {
		return false
	}
	for _, account := range accounts {
		if !account.EphemeralAPIKey || !strings.EqualFold(strings.TrimSpace(account.Provider.Kind), provider) {
			return false
		}
	}
	return true
}

func rawKeyLocalRoute(request proxymodel.Request) bool {
	path := strings.TrimRight(request.Path, "/")
	return request.QuotaSelection.RouteKind == quotamodel.RouteKindCompact || request.Method == http.MethodGet &&
		(strings.HasSuffix(path, "/models") || strings.Contains(path, "/models/"))
}
