package routing

import (
	"context"
	"net/http"
	"strconv"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type routeHealthKey struct {
	accountID string
	route     string
}

func (router *Router) recordRouteFailure(ctx context.Context, accountID string, selection quotamodel.Selection) {
	router.recordRouteFailurePenaltyReason(ctx, accountID, selection, 1, "route_failure")
}

func (router *Router) recordTransportExecutionFailure(
	ctx context.Context, accountID string, selection quotamodel.Selection, err error,
) {
	penalty := otherTransportHealthPenalty
	if err != nil {
		penalty = transportHealthPenalty(err.Error())
	}
	router.bumpRouteBadPairing(ctx, accountID, selection, badPairingPenalty)
	router.recordRouteFailurePenaltyReason(ctx, accountID, selection, penalty, "transport_failure")
	router.recordRouteLatencyFailure(context.Background(), accountID, selection, "transport")
	router.persistTransportBackoff(ctx, accountID, selection)
}

func (router *Router) recordRouteFailurePenalty(ctx context.Context, accountID string, selection quotamodel.Selection, penalty uint8) {
	reason := routeHealthRoute(selection.RouteKind) + "_overload"
	router.bumpRouteBadPairing(ctx, accountID, selection, badPairingPenalty)
	router.recordRouteFailurePenaltyReason(ctx, accountID, selection, penalty, reason)
}

func (router *Router) recordRouteFailurePenaltyReason(ctx context.Context, accountID string, selection quotamodel.Selection, penalty uint8, reason string) {
	if penalty == 0 {
		return
	}
	router.resetRouteSuccessStreak(ctx, accountID, selection)
	for range penalty {
		router.adjustRouteHealth(ctx, accountID, selection, 1)
	}
	route := routeHealthRoute(selection.RouteKind)
	router.mu.Lock()
	score := router.routeHealth[routeHealthKey{accountID: accountID, route: route}].Effective(router.now())
	router.mu.Unlock()
	router.recordRuntimeMarker(ctx, runtimemodel.Event{Kind: "profile_health", Fields: map[string]string{
		"profile": accountID, "route": route, "score": strconv.Itoa(int(score)),
		"delta": strconv.Itoa(int(penalty)), "reason": reason,
	}})
	router.openRouteCircuit(ctx, accountID, selection)
}

func (router *Router) recordRouteSuccess(ctx context.Context, accountID string, selection quotamodel.Selection) {
	now := router.now()
	key := routeHealthKey{accountID: accountID, route: routeHealthRoute(selection.RouteKind)}
	router.mu.Lock()
	current := router.routeHealth[key].Effective(now)
	router.mu.Unlock()
	if current > 0 {
		streak := router.routeSuccessStreak(accountID, selection, now)
		next, nextStreak, retain := routeHealthRecovery(current, streak)
		router.setRouteHealth(ctx, accountID, selection, next)
		if retain {
			router.setRouteSuccessStreak(ctx, accountID, selection, nextStreak)
		} else {
			router.resetRouteSuccessStreak(ctx, accountID, selection)
		}
	} else {
		router.resetRouteSuccessStreak(ctx, accountID, selection)
	}
	router.clearRouteCircuit(ctx, accountID, selection)
}

func (router *Router) recordRouteOutcome(
	ctx context.Context,
	accountID string,
	selection quotamodel.Selection,
	response *proxymodel.Response,
	outcome responseOutcome,
) {
	if ctx.Err() != nil || response == nil {
		return
	}
	transportFailure := outcome.transport || (response.PrecommitFailure != nil && response.PrecommitFailure.Transport)
	if transportFailure {
		router.persistTransportBackoff(ctx, accountID, selection)
		router.recordRouteLatencyFailure(context.Background(), accountID, selection, "transport")
	}
	if outcome.kind == responsePass {
		if outcome.healthPenalty > 0 {
			router.recordRouteFailurePenalty(ctx, accountID, selection, outcome.healthPenalty)
		} else if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusBadRequest {
			if !outcome.failed {
				router.clearRetryBackoff(ctx, accountID)
				router.clearTransportBackoff(ctx, accountID, selection)
			}
			router.recordRouteSuccess(ctx, accountID, selection)
		}
		return
	}
	if outcome.healthPenalty > 0 {
		router.recordRouteFailurePenalty(ctx, accountID, selection, outcome.healthPenalty)
	}
}

func (router *Router) setRouteHealth(ctx context.Context, accountID string, selection quotamodel.Selection, value uint8) {
	if ctx.Err() != nil || accountID == "" {
		return
	}
	route := routeHealthRoute(selection.RouteKind)
	if route == "" {
		return
	}
	key := routeHealthKey{accountID: accountID, route: route}
	now := router.now()
	if router.state != nil && router.persistenceWritesEnabled() {
		router.routeHealthMu.Lock()
		updated, err := router.state.SetRouteHealth(ctx, accountID, route, value, now)
		router.routeHealthMu.Unlock()
		if err == nil {
			router.mu.Lock()
			if value == 0 {
				delete(router.routeHealth, key)
			} else {
				router.routeHealth[key] = updated
			}
			router.mu.Unlock()
			return
		}
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if value == 0 {
		delete(router.routeHealth, key)
		return
	}
	router.routeHealth[key] = routingentity.RouteHealthScore{AccountID: accountID, Route: route, Score: value, UpdatedUnix: now.Unix()}
}

func (router *Router) adjustRouteHealth(ctx context.Context, accountID string, selection quotamodel.Selection, delta int) {
	if ctx.Err() != nil || accountID == "" {
		return
	}
	route := routeHealthRoute(selection.RouteKind)
	if route == "" {
		return
	}
	key := routeHealthKey{accountID: accountID, route: route}
	if delta < 0 {
		router.mu.Lock()
		current := router.routeHealth[key]
		noPenalty := current.Effective(router.now()) == 0
		router.mu.Unlock()
		if noPenalty {
			return
		}
	}
	if router.state != nil && router.persistenceWritesEnabled() {
		router.routeHealthMu.Lock()
		updated, err := router.state.AdjustRouteHealth(ctx, accountID, route, delta, router.now())
		if err == nil {
			router.mu.Lock()
			router.routeHealth[key] = updated
			router.mu.Unlock()
			router.routeHealthMu.Unlock()
			return
		}
		router.routeHealthMu.Unlock()
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.routeHealth == nil {
		router.routeHealth = make(map[routeHealthKey]routingentity.RouteHealthScore)
	}
	current, exists := router.routeHealth[key]
	if !exists && len(router.routeHealth) >= maxRouteHealthScores {
		return
	}
	if !exists {
		current = routingentity.RouteHealthScore{AccountID: accountID, Route: route}
	}
	updated, err := current.Adjust(delta, router.now())
	if err == nil {
		router.routeHealth[key] = updated
	}
}

func routeHealthRoute(route quotamodel.RouteKind) string {
	switch route {
	case quotamodel.RouteKindStandard:
		return "standard"
	case quotamodel.RouteKindResponses:
		return "responses"
	case quotamodel.RouteKindCompact:
		return "compact"
	case quotamodel.RouteKindWebSocket:
		return "websocket"
	default:
		return ""
	}
}
