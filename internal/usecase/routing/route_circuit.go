package routing

import (
	"context"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) openRouteCircuit(ctx context.Context, accountID string, selection quotamodel.Selection) {
	route := routeHealthRoute(selection.RouteKind)
	if ctx.Err() != nil || accountID == "" || route == "" {
		return
	}
	now := router.now()
	key := routeHealthKey{accountID: accountID, route: route}
	router.routeCircuitMu.Lock()
	defer router.routeCircuitMu.Unlock()

	router.mu.Lock()
	healthScore := router.routeHealth[key].Effective(now)
	previous, exists := router.routeCircuits[key]
	router.mu.Unlock()
	var previousCircuit *routingentity.RouteCircuit
	if exists {
		previousCircuit = &previous
	}
	if router.state != nil && router.persistenceWritesEnabled() {
		circuit, opened, err := router.state.OpenRouteCircuit(ctx, accountID, route, healthScore, now)
		if err == nil {
			if opened {
				router.mu.Lock()
				router.routeCircuits[key] = circuit
				router.mu.Unlock()
			}
			return
		}
	}
	circuit, opened := routingentity.OpenRouteCircuit(accountID, route, healthScore, previousCircuit, now)
	if !opened {
		return
	}
	router.mu.Lock()
	router.routeCircuits[key] = circuit
	router.mu.Unlock()
}

func (router *Router) clearRouteCircuit(ctx context.Context, accountID string, selection quotamodel.Selection) {
	route := routeHealthRoute(selection.RouteKind)
	if ctx.Err() != nil || accountID == "" || route == "" {
		return
	}
	key := routeHealthKey{accountID: accountID, route: route}
	router.routeCircuitMu.Lock()
	defer router.routeCircuitMu.Unlock()
	router.mu.Lock()
	_, exists := router.routeCircuits[key]
	if exists {
		delete(router.routeCircuits, key)
	}
	router.mu.Unlock()
	if exists && router.state != nil && router.persistenceWritesEnabled() {
		_ = router.state.ClearRouteCircuit(ctx, accountID, route)
	}
}

func (router *Router) routeCircuitRemaining(accountID string, selection quotamodel.Selection, now time.Time) time.Duration {
	route := routeHealthRoute(selection.RouteKind)
	if accountID == "" || route == "" {
		return 0
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	circuit, exists := router.routeCircuits[routeHealthKey{accountID: accountID, route: route}]
	if !exists {
		return 0
	}
	return circuit.Remaining(now)
}

func (router *Router) reserveRouteCircuitProbe(
	ctx context.Context,
	accountID string,
	selection quotamodel.Selection,
	now time.Time,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	route := routeHealthRoute(selection.RouteKind)
	if accountID == "" || route == "" {
		return true, nil
	}
	key := routeHealthKey{accountID: accountID, route: route}
	router.routeCircuitMu.Lock()
	defer router.routeCircuitMu.Unlock()
	router.mu.Lock()
	circuit, exists := router.routeCircuits[key]
	healthScore := router.routeHealth[key].Effective(now)
	router.mu.Unlock()
	if !exists {
		return true, nil
	}
	if router.state != nil && router.persistenceWritesEnabled() {
		updated, allowed, err := router.state.ReserveRouteCircuitProbe(ctx, accountID, route, healthScore, now)
		if err != nil {
			return false, err
		}
		router.mu.Lock()
		if updated.UntilUnix == 0 {
			delete(router.routeCircuits, key)
		} else {
			router.routeCircuits[key] = updated
		}
		router.mu.Unlock()
		return allowed, nil
	}
	updated, allowed, changed := circuit.ReserveProbe(healthScore, now)
	if changed {
		router.mu.Lock()
		if updated.UntilUnix == 0 {
			delete(router.routeCircuits, key)
		} else {
			router.routeCircuits[key] = updated
		}
		router.mu.Unlock()
	}
	return allowed, nil
}
