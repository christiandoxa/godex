package routing

import (
	"context"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// An opaque WebSocket chain with no verified owner may select one profile
// for discovery, as tagged Prodex 0.437.9 does. The gateway's bounded
// previous-response retry planner can repeat the *same-owner* attempt,
// but cannot drop tool-output continuation context. Prodex 0.438.3 also
// permits precommit transport/quota recovery with independently complete history.
func (router *Router) forwardUnboundWebSocketDiscovery(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	keys *affinityKeys,
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
			transportFailure := pending.transport
			if response := pending.response; response != nil && response.PrecommitFailure != nil && !response.FirstEventCommitted {
				transportFailure = transportFailure || response.PrecommitFailure.Transport
			}
			if recovered, handled, err := router.recoverWebSocketTurnStateFullHistory04383(
				ctx, request, accounts, account, routedWebSocketRequestMetadata(request),
				responseOutcome{kind: responseRetry, transport: transportFailure, quota: pending.quota}, pending,
			); handled || err != nil {
				if keys != nil && err == nil {
					keys.turn = ""
				}
				return recovered, err
			}
			return finishFresh(&pending)
		}
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "unbound websocket discovery produced no response",
		}
	}
}
