package routing

import (
	"context"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) rotateTurnOrSessionOnlyWebSocketPreviousResponse(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
	outcome responseOutcome,
	pending *pendingResponse,
) (proxymodel.Forwarded, bool, error) {
	// A strict Compact followup has an existing continuation owner. Unlike
	// an ordinary turn-only Responses request, a missing upstream response
	// does not authorize replaying its input on another profile.
	if !outcome.previousResponseNotFound || request.QuotaSelection.RouteKind == quotamodel.RouteKindCompact {
		return proxymodel.Forwarded{}, false, nil
	}
	metadata := routedWebSocketRequestMetadata(request)
	if metadata.previousResponseID != "" ||
		(metadata.turnState == "" && metadata.sessionID == "") {
		return proxymodel.Forwarded{}, false, nil
	}
	// No previous ID was supplied, so its error does not invalidate a hard turn.
	if metadata.turnState == "" {
		if err := router.affinity.forgetOwned(
			ctx, account.ID, affinityKeys{session: metadata.sessionID}, router.now(),
		); err != nil {
			closePendingResponse(pending)
			return proxymodel.Forwarded{}, true, err
		}
	}

	fallback := make([]proxymodel.Account, 0, len(accounts))
	for _, candidate := range accounts {
		if candidate.ID != account.ID {
			fallback = append(fallback, candidate)
		}
	}
	if len(fallback) == 0 {
		return pendingForwarded(account.ID, outcome, pending), true, nil
	}
	closePendingResponse(pending)
	result, err := router.forwardFresh(ctx, request, fallback)
	return result, true, err
}
