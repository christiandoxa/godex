package routing

import (
	"context"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// Prodex 0.438.3 shares the tagged full-history proof between quota and
// precommit transport recovery. A failed turn-state-only connection can
// hand off a reconstructable full transcript to a different Ready profile,
// after releasing any matching failed-owner binding. This must never
// replay previous_response_id dependencies, delta-only tool output, compact
// followups, another profile's owned turn-state, or committed model output.
func (router *Router) recoverWebSocketTurnStateFullHistory04383(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	failed proxymodel.Account,
	metadata websocketRequestMetadata,
	outcome responseOutcome,
	pending *pendingResponse,
) (proxymodel.Forwarded, bool, error) {
	if !request.WebSocketMessage || outcome.kind != responseRetry || (!outcome.transport && !outcome.quota) ||
		metadata.previousResponseID != "" || metadata.turnState == "" ||
		request.WebSocketPolicy.RealtimeDuplex || request.FirstEventRetryUsed ||
		request.QuotaSelection.RouteKind == quotamodel.RouteKindCompact ||
		request.WebSocketPolicy.RequestPreviousResponse ||
		!requestHasReconstructableFullHistory(request) {
		return proxymodel.Forwarded{}, false, nil
	}
	// Full history may wait for capacity; quota/auth failures still exclude
	// the alternate under the existing full-context recovery policy.
	var fallback []proxymodel.Account
	for _, candidate := range accounts {
		if router.websocketQuotaFullContextFallbackRecoverable(request, []proxymodel.Account{candidate}, failed) {
			fallback = append(fallback, candidate)
		}
	}
	if len(fallback) == 0 {
		return proxymodel.Forwarded{}, false, nil
	}
	owner, err := router.affinity.owner(ctx, affinityKeys{turn: metadata.turnState}, router.now())
	if err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, true, err
	}
	if owner != "" && owner != failed.ID {
		return proxymodel.Forwarded{}, false, nil
	}
	// The tagged release plan retains session ownership beside a hard turn.
	if err := router.affinity.releaseOwnedDead(ctx, failed.ID,
		affinityKeys{turn: metadata.turnState}, router.now()); err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, true, err
	}
	closePendingResponse(pending)
	clean := requestWithoutTurnState(request)
	// Keep the full body/history and original logical request deadline;
	// never forward the failed transport's stale turn token to another home.
	recovered, err := router.forwardFresh(ctx, clean, fallback)
	return recovered, true, err
}
