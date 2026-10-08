package routing

import (
	"context"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) resolveOwner(
	ctx context.Context,
	keys affinityKeys,
	selection quotamodel.Selection,
) (string, func() error, error) {
	owner, err := router.affinity.owner(ctx, keys, router.now())
	if err != nil {
		return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
	}
	var durableRelease func() error
	if owner == "" && router.affinity.repository != nil && stableConversation(keys) {
		durableRelease, err = router.affinity.repository.AcquireConversation(ctx)
		if err != nil {
			return "", nil, err
		}
		owner, err = router.affinity.owner(ctx, keys, router.now())
		if err != nil {
			_ = durableRelease()
			return "", nil, err
		}
	}
	if owner == "" && opaqueContinuation(keys, selection) &&
		(keys.previous == "" || !router.hasPreviousResponseFailure(keys.previous, selection)) {
		if durableRelease != nil {
			_ = durableRelease()
		}
		return "", nil, &proxymodel.Error{StatusCode: 409, Message: "continuation owner is unknown; continuity was preserved"}
	}
	return owner, durableRelease, nil
}

func stableConversation(keys affinityKeys) bool {
	return keys.thread != "" || keys.session != ""
}

func opaqueContinuation(keys affinityKeys, selection quotamodel.Selection) bool {
	if keys.previous != "" {
		return true
	}
	return keys.turn != "" && selection.RouteKind != quotamodel.RouteKindCompact
}

func (router *Router) routeRequest(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	owner string,
	keys *affinityKeys,
) (proxymodel.Forwarded, error) {
	if owner != "" {
		if keys != nil && keys.hasSoftSessionAffinity(request.QuotaSelection) &&
			router.softSessionOwnerBlocked(accounts, owner, request.QuotaSelection) {
			return router.forwardFresh(ctx, request, accounts)
		}
		return router.forwardBound(ctx, request, accounts, owner, keys)
	}
	return router.forwardFresh(ctx, request, accounts)
}
