package routing

import (
	"context"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (router *Router) rotateTurnOrSessionOnlyWebSocketPreviousResponse(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
	outcome responseOutcome,
	pending *pendingResponse,
) (proxymodel.Forwarded, bool, error) {
	if !outcome.previousResponseNotFound {
		return proxymodel.Forwarded{}, false, nil
	}
	metadata := routedWebSocketRequestMetadata(request)
	if metadata.previousResponseID != "" ||
		(metadata.turnState == "" && metadata.sessionID == "") {
		return proxymodel.Forwarded{}, false, nil
	}
	if err := router.affinity.forgetOwned(
		ctx,
		account.ID,
		affinityKeys{turn: metadata.turnState, session: metadata.sessionID},
		router.now(),
	); err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, true, err
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
