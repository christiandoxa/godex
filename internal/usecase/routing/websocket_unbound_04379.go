package routing

import (
	"context"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// An opaque WebSocket chain with no verified owner may select one profile
// for discovery, as tagged Prodex 0.437.9 does. The gateway's bounded
// previous-response retry planner can repeat the *same-owner* attempt,
// but this is never permission to replay on another profile or drop
// tool-output continuation context.
func (router *Router) forwardUnboundWebSocketDiscovery(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
) (proxymodel.Forwarded, error) {
	candidates := router.requestCandidatesForRequest(accounts, request, router.now())
	if len(candidates) == 0 {
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "no eligible profile for unbound websocket continuation",
		}
	}
	account := candidates[0]
	allowed, err := router.reserveRouteCircuitProbe(ctx, account.ID, request.QuotaSelection, router.now())
	if err != nil {
		return proxymodel.Forwarded{}, err
	}
	if !allowed {
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "unbound websocket profile is temporarily unavailable",
		}
	}
	for {
		router.recordSelectionMarker(ctx, "selection_pick", account, request.QuotaSelection)
		router.recordRouteDecisionSelected(ctx, request, account)
		result, pending, saturated, err := router.freshAttempt(ctx, request, account, false)
		if err != nil {
			return proxymodel.Forwarded{}, err
		}
		if saturated {
			if err := router.waitForProfileInflight(ctx, request, []proxymodel.Account{account}); err != nil {
				return proxymodel.Forwarded{}, err
			}
			continue
		}
		if result != nil {
			return *result, nil
		}
		if pending != nil {
			// The first profile has already seen this opaque continuation.
			// Never select a second profile or convert a tool result into
			// a fresh context-less request after a precommit failure.
			return finishFresh(&pending)
		}
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "unbound websocket discovery produced no response",
		}
	}
}
