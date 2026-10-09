package routing

import (
	"context"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) mutateRouteMemory(
	ctx context.Context, accountID string, selection quotamodel.Selection, kind string,
	mutation func(routingentity.RouteMemoryScore) routingentity.RouteMemoryScore,
) routingentity.RouteMemoryScore {
	route := routeHealthRoute(selection.RouteKind)
	if ctx.Err() != nil || accountID == "" || route == "" || mutation == nil {
		return routingentity.RouteMemoryScore{}
	}
	now := router.now()
	key := routeMemoryKey{accountID: accountID, route: route, kind: kind}
	if router.state != nil && router.persistAccountState(accountID) {
		router.routeMemoryMu.Lock()
		updated, err := router.state.MutateRouteMemory(ctx, accountID, route, kind, now, mutation)
		router.routeMemoryMu.Unlock()
		if err == nil {
			router.mu.Lock()
			if updated.Score == 0 {
				delete(router.routeMemory, key)
			} else {
				router.routeMemory[key] = updated
			}
			router.mu.Unlock()
			return updated
		}
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	current := router.routeMemory[key]
	if current.AccountID == "" {
		current = routingentity.RouteMemoryScore{AccountID: accountID, Route: route, Kind: kind, UpdatedUnix: now.Unix()}
	}
	updated := mutation(current)
	updated.AccountID, updated.Route, updated.Kind, updated.UpdatedUnix = accountID, route, kind, now.Unix()
	if updated.Score == 0 {
		delete(router.routeMemory, key)
	} else if updated.Validate() == nil {
		router.routeMemory[key] = updated
	}
	return updated
}

func (router *Router) resetRouteSuccessStreak(ctx context.Context, accountID string, selection quotamodel.Selection) {
	router.mutateRouteMemory(ctx, accountID, selection, routingentity.RouteMemorySuccessStreak, func(score routingentity.RouteMemoryScore) routingentity.RouteMemoryScore {
		score.Score = 0
		return score
	})
}

func (router *Router) bumpRouteBadPairing(ctx context.Context, accountID string, selection quotamodel.Selection, delta uint8) {
	if delta == 0 {
		return
	}
	router.resetRouteSuccessStreak(ctx, accountID, selection)
	router.mutateRouteMemory(ctx, accountID, selection, routingentity.RouteMemoryBadPairing, func(score routingentity.RouteMemoryScore) routingentity.RouteMemoryScore {
		current := score.Effective(router.now())
		score.Score = min(uint8(routingentity.MaxRouteHealthScore), current+delta)
		return score
	})
}

func (router *Router) routeSuccessStreak(accountID string, selection quotamodel.Selection, now time.Time) uint8 {
	key := routeMemoryKey{accountID: accountID, route: routeHealthRoute(selection.RouteKind), kind: routingentity.RouteMemorySuccessStreak}
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.routeMemory[key].Effective(now)
}

func (router *Router) setRouteSuccessStreak(ctx context.Context, accountID string, selection quotamodel.Selection, value uint8) {
	router.mutateRouteMemory(ctx, accountID, selection, routingentity.RouteMemorySuccessStreak, func(score routingentity.RouteMemoryScore) routingentity.RouteMemoryScore {
		score.Score = min(value, uint8(3))
		return score
	})
}
