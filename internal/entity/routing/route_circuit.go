package routing

import (
	"errors"
	"time"
)

const (
	// RouteCircuitHealthThreshold opens a route after repeated failures.
	RouteCircuitHealthThreshold uint8 = 4
	// RouteCircuitMaxReopenStage bounds repeated circuit extensions.
	RouteCircuitMaxReopenStage uint8 = 4
	// RouteCircuitOpenDuration is the initial route-circuit cooldown.
	RouteCircuitOpenDuration = 20 * time.Second
	// RouteCircuitMaxOpenDuration caps repeated route-circuit cooldowns.
	RouteCircuitMaxOpenDuration = 10 * time.Minute
	// RouteCircuitProbeDuration is the initial half-open probe lease.
	RouteCircuitProbeDuration = 5 * time.Second
	// RouteCircuitMaxProbeDuration caps half-open probe leases.
	RouteCircuitMaxProbeDuration = time.Minute
	// RouteCircuitReopenDecay resets reopen escalation after sustained recovery.
	RouteCircuitReopenDecay = 30 * time.Minute
)

// RouteCircuit stores one account's temporary exclusion for a route.
type RouteCircuit struct {
	AccountID        string `json:"account_id"`
	Route            string `json:"route"`
	UntilUnix        int64  `json:"until_unix"`
	ReopenStage      uint8  `json:"reopen_stage"`
	StageUpdatedUnix int64  `json:"stage_updated_unix"`
}

func (circuit RouteCircuit) Validate() error {
	if !validAccountID(circuit.AccountID) || !validRoute(circuit.Route) ||
		circuit.UntilUnix <= 0 || circuit.ReopenStage > RouteCircuitMaxReopenStage ||
		circuit.StageUpdatedUnix < 0 {
		return errors.New("invalid routing circuit")
	}
	return nil
}

// OpenRouteCircuit applies the route health threshold and bounded reopen delay.
func OpenRouteCircuit(accountID, route string, healthScore uint8, previous *RouteCircuit, now time.Time) (RouteCircuit, bool) {
	if healthScore < RouteCircuitHealthThreshold || !validAccountID(accountID) || !validRoute(route) {
		return RouteCircuit{}, false
	}
	stage := uint8(0)
	if previous != nil && previous.UntilUnix > 0 &&
		now.Unix()-previous.StageUpdatedUnix < int64(RouteCircuitReopenDecay/time.Second) {
		stage = min(previous.ReopenStage+1, RouteCircuitMaxReopenStage)
	}
	exponent := min(int(healthScore-RouteCircuitHealthThreshold), 3) + int(stage)
	openFor := min(RouteCircuitOpenDuration*time.Duration(1<<exponent), RouteCircuitMaxOpenDuration)
	return RouteCircuit{
		AccountID: accountID, Route: route, UntilUnix: now.Add(openFor).Unix(),
		ReopenStage: stage, StageUpdatedUnix: now.Unix(),
	}, true
}

// Remaining returns the circuit's active cooldown.
func (circuit RouteCircuit) Remaining(now time.Time) time.Duration {
	remaining := time.Unix(circuit.UntilUnix, 0).Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ReserveProbe reserves a single half-open probe or clears recovered state.
func (circuit RouteCircuit) ReserveProbe(healthScore uint8, now time.Time) (RouteCircuit, bool, bool) {
	if circuit.UntilUnix > now.Unix() {
		return circuit, false, false
	}
	if healthScore == 0 {
		return RouteCircuit{}, true, true
	}
	probeFor := routeCircuitProbeDuration(healthScore)
	circuit.UntilUnix = now.Add(probeFor).Unix()
	return circuit, true, true
}

// SoftenForRestart shortens active cooldowns after process restart.
func (circuit RouteCircuit) SoftenForRestart(healthScore uint8, now time.Time) (RouteCircuit, bool, bool) {
	if circuit.UntilUnix <= now.Unix() {
		return RouteCircuit{}, false, true
	}
	changed := false
	if now.Unix() >= circuit.StageUpdatedUnix &&
		now.Unix()-circuit.StageUpdatedUnix >= int64(RouteCircuitReopenDecay/time.Second) && circuit.ReopenStage != 0 {
		circuit.ReopenStage = 0
		circuit.StageUpdatedUnix = now.Unix()
		changed = true
	}
	probeFor := routeCircuitProbeDuration(healthScore)
	maxUntil := now.Add(probeFor).Unix()
	if circuit.UntilUnix > maxUntil {
		circuit.UntilUnix = maxUntil
		changed = true
	}
	return circuit, true, changed
}

func routeCircuitProbeDuration(healthScore uint8) time.Duration {
	exponent := 0
	if healthScore > RouteCircuitHealthThreshold {
		exponent = min(int(healthScore-RouteCircuitHealthThreshold), 3)
	}
	return min(RouteCircuitProbeDuration*time.Duration(1<<exponent), RouteCircuitMaxProbeDuration)
}
