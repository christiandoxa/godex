package routing

import (
	"strings"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const (
	badPairingPenalty             = uint8(2)
	connectTransportHealthPenalty = uint8(5)
	otherTransportHealthPenalty   = uint8(4)
)

type routeMemoryKey struct {
	accountID string
	route     string
	kind      string
}

func coupledRoute(route string) string {
	switch route {
	case "responses":
		return "websocket"
	case "websocket":
		return "responses"
	case "compact":
		return "standard"
	case "standard":
		return "compact"
	default:
		return ""
	}
}

func (router *Router) routeCompositeHealthScore(accountID, route string, now time.Time) uint32 {
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.routeCompositeHealthScoreLocked(accountID, route, now)
}

func (router *Router) routeCompositeHealthScoreLocked(accountID, route string, now time.Time) uint32 {
	globalHealth := uint32(router.routeHealth[routeHealthKey{accountID: accountID, route: "global"}].Effective(now))
	ownHealth := uint32(router.routeHealth[routeHealthKey{accountID: accountID, route: route}].Effective(now))
	ownBad := uint32(router.routeMemory[routeMemoryKey{accountID: accountID, route: route, kind: routingentity.RouteMemoryBadPairing}].Effective(now))
	ownPerf := uint32(router.routeMemory[routeMemoryKey{accountID: accountID, route: route, kind: routingentity.RouteMemoryPerformance}].Effective(now))
	coupled := coupledRoute(route)
	coupledHealth := uint32(router.routeHealth[routeHealthKey{accountID: accountID, route: coupled}].Effective(now))
	coupledBad := uint32(router.routeMemory[routeMemoryKey{accountID: accountID, route: coupled, kind: routingentity.RouteMemoryBadPairing}].Effective(now))
	coupledPerf := uint32(router.routeMemory[routeMemoryKey{accountID: accountID, route: coupled, kind: routingentity.RouteMemoryPerformance}].Effective(now))
	return saturatingHealthAdd(globalHealth, ownHealth, ownBad, (coupledHealth+coupledBad)/2, ownPerf, coupledPerf/2)
}

func saturatingHealthAdd(values ...uint32) uint32 {
	var total uint32
	for _, value := range values {
		if ^uint32(0)-total < value {
			return ^uint32(0)
		}
		total += value
	}
	return total
}

func healthLatencyPenalty(elapsedMS uint64, route quotamodel.RouteKind, stage string) uint8 {
	good, warn, poor, severe := uint64(100), uint64(250), uint64(600), uint64(1200)
	if (route == quotamodel.RouteKindResponses && stage == "ttfb") || (route == quotamodel.RouteKindWebSocket && stage == "connect") {
		good, warn, poor, severe = 120, 300, 700, 1500
	} else if route == quotamodel.RouteKindCompact || route == quotamodel.RouteKindStandard {
		good, warn, poor, severe = 80, 180, 400, 900
	}
	switch {
	case elapsedMS <= good:
		return 0
	case elapsedMS <= warn:
		return 2
	case elapsedMS <= poor:
		return 4
	case elapsedMS <= severe:
		return 7
	default:
		return 12
	}
}

func healthLatencyNextScore(current, observed uint8) uint8 {
	if observed == 0 {
		if current > 2 {
			return current - 2
		}
		return 0
	}
	return uint8((uint16(current)*2 + uint16(observed) + 2) / 3)
}

func healthLatencyFailureNextScore(current uint8) uint8 {
	return min(uint8(12), current+connectTransportHealthPenalty)
}

func routeHealthRecovery(current, streak uint8) (nextScore, nextStreak uint8, retain bool) {
	nextStreak = min(streak+1, uint8(3))
	extra := uint8(0)
	if nextStreak > 1 {
		extra = 1
	}
	recovery := uint8(2) + extra
	if current <= recovery {
		return 0, 0, false
	}
	return current - recovery, nextStreak, true
}

func transportHealthPenalty(code string) uint8 {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, marker := range []string{"dns", "connect_timeout", "connection refused", "connect_refused", "connection reset", "connect_reset", "tls", "handshake", "certificate"} {
		if strings.Contains(code, marker) {
			return connectTransportHealthPenalty
		}
	}
	return otherTransportHealthPenalty
}
