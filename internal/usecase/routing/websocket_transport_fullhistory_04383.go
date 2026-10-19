package routing

import (
	"context"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// Prodex 0.438.3 shares the tagged full-history proof between quota and
// *precommit transport* recovery. A failed turn-state-only connection can
// hand off a reconstructable full transcript to a different Ready profile,
// after releasing only the failed owner's exact binding. This must never
// replay previous_response_id dependencies, delta-only tool output, compact
// followups, an unowned turn-state, or already committed model output.
func (router *Router) recoverWebSocketTurnStateTransport04383(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	failed proxymodel.Account,
	metadata websocketRequestMetadata,
	outcome responseOutcome,
	pending *pendingResponse,
) (proxymodel.Forwarded, bool, error) {
	if !request.WebSocketMessage || outcome.kind != responseRetry || !outcome.transport ||
		metadata.previousResponseID != "" || metadata.turnState == "" ||
		request.WebSocketPolicy.RealtimeDuplex || request.FirstEventRetryUsed ||
		request.QuotaSelection.RouteKind == quotamodel.RouteKindCompact ||
		request.WebSocketPolicy.RequestPreviousResponse ||
		!requestHasReconstructableFullHistory(request) {
		return proxymodel.Forwarded{}, false, nil
	}
	// A ready alternate is required. Never rotate into a quarantined,
	// exhausted, unregistered, unrelated-provider or capacity-full owner.
	decision := router.websocketQuotaFallbackDecisionWithContext(request, accounts, failed, true)
	if decision.kind != websocketQuotaFallbackReady {
		return proxymodel.Forwarded{}, false, nil
	}
	// A cached authenticated negative quota receipt also excludes the
	// candidate. An idle permit is not sufficient evidence of model quota.
	if cachedState, present := router.cachedQuotaCheck(decision.account.ID, request.QuotaSelection, router.now()); present &&
		!cachedState.ready && (cachedState.retryAt.IsZero() || cachedState.retryAt.After(router.now())) {
		return proxymodel.Forwarded{}, false, nil
	}
	owned, err := router.turnStateOwnedBy(ctx, metadata.turnState, failed.ID)
	if err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, true, err
	}
	if !owned {
		return proxymodel.Forwarded{}, false, nil
	}
	if err := router.affinity.releaseOwnedDead(ctx, failed.ID,
		affinityKeys{turn: metadata.turnState, session: metadata.sessionID}, router.now()); err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, true, err
	}
	closePendingResponse(pending)
	clean := requestWithoutTurnState(request)
	// Keep the full body/history and original logical request deadline;
	// never forward the failed transport's stale turn token to another home.
	recovered, err := router.forwardFresh(ctx, clean, []proxymodel.Account{decision.account})
	return recovered, true, err
}
