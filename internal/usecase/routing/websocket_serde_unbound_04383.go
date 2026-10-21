package routing

import (
	"context"
	"net/http"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	// These are the published, non-test Prodex runtime-proxy precommit
	// attempt and wall-clock budgets in failure_response.rs.
	websocketSerdeUnboundAttemptLimit04383 = 12
	websocketSerdeUnboundBudget04383       = 3 * time.Second
)

// A WebSocket text request which failed complete serde parsing cannot
// assert a verified hard previous-response owner. Once a candidate replies
// with precommit previous_response_not_found, tagged Prodex reselects an
// eligible profile rather than spending the locked-owner retry schedule or
// simply forwarding the first 400. Keep an isolated bounded request-local
// retry budget; do not persist an owner from the malformed body.
func (router *Router) forwardWebSocketInvalidSerdeUnbound04383(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
) (proxymodel.Forwarded, error) {
	candidates := router.requestCandidatesForRequest(accounts, request, router.now())
	if len(candidates) == 0 {
		return router.forwardFresh(ctx, request, accounts)
	}
	started := time.Now()
	attempts := 0
	var lastAccount string
	for attempts < websocketSerdeUnboundAttemptLimit04383 {
		if err := ctx.Err(); err != nil {
			return proxymodel.Forwarded{}, err
		}
		account := candidates[attempts%len(candidates)]
		router.recordSelectionMarker(ctx, "selection_pick", account, request.QuotaSelection)
		router.recordRouteDecisionSelected(ctx, request, account)
		result, found, pending, saturated, err := router.tryFreshCandidate(ctx, request, candidates, account)
		if err != nil {
			return proxymodel.Forwarded{}, err
		}
		if saturated {
			if err := router.waitForProfileInflight(ctx, request, candidates); err != nil {
				return proxymodel.Forwarded{}, err
			}
			continue // Capacity wait is not an upstream inference attempt.
		}
		attempts++
		lastAccount = account.ID
		if found {
			if !websocketPreviousResponseNotFound(result.Response) {
				return result, nil
			}
			if attempts >= websocketSerdeUnboundAttemptLimit04383 ||
				time.Since(started) >= websocketSerdeUnboundBudget04383 {
				translated := staleGenericWebSocketContinuationResponse(result.Response)
				return proxymodel.Forwarded{Response: translated, AccountID: lastAccount, Failed: true}, nil
			}
			closeWebSocketRoutingResponse(result.Response)
			continue
		}
		if pending != nil {
			// Only previous-response-not-found is the observed unbound
			// recovery reason for this path. Preserve the usual fresh
			// classification for quota, auth and unrelated transport errors.
			defer pending.close()
			return finishFresh(&pending)
		}
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "unbound WebSocket request returned no upstream response",
		}
	}
	return proxymodel.Forwarded{}, &proxymodel.Error{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "unbound WebSocket precommit attempts were exhausted",
	}
}
