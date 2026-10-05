package routing

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

func (store *Store) LoadRouteCircuits(
	ctx context.Context,
	now time.Time,
	healthScores []routingentity.RouteHealthScore,
) ([]routingentity.RouteCircuit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, fmt.Errorf("prepare routing circuit store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-health.guard"))
	if err != nil {
		return nil, fmt.Errorf("lock routing circuit store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot, err := store.readRouteCircuits()
	if err != nil {
		return nil, fmt.Errorf("read routing circuit store: %w", err)
	}
	scores := make(map[string]routingentity.RouteHealthScore, len(healthScores))
	for _, score := range healthScores {
		scores[score.AccountID+"\x00"+score.Route] = score
	}
	cutoff := now.Add(-routingentity.RouteHealthRetention).Unix()
	active := make([]routingentity.RouteCircuit, 0, len(snapshot.Circuits))
	changed := false
	for _, circuit := range snapshot.Circuits {
		if circuit.StageUpdatedUnix <= cutoff {
			changed = true
			continue
		}
		score := scores[circuit.AccountID+"\x00"+circuit.Route].Effective(now)
		softened, keep, softenedChanged := circuit.SoftenForRestart(score, now)
		if !keep {
			changed = true
			continue
		}
		active = append(active, softened)
		changed = changed || softenedChanged
	}
	active = retainRouteCircuits(active, now)
	if changed || len(active) != len(snapshot.Circuits) {
		if err := store.writeRouteCircuits(active); err != nil {
			return nil, fmt.Errorf("prune routing circuit store: %w", err)
		}
	}
	return active, nil
}

func (store *Store) ReserveRouteCircuitProbe(
	ctx context.Context,
	accountID, route string,
	healthScore uint8,
	now time.Time,
) (routingentity.RouteCircuit, bool, error) {
	if err := ctx.Err(); err != nil {
		return routingentity.RouteCircuit{}, false, err
	}
	if err := (routingentity.RouteCircuit{AccountID: accountID, Route: route, UntilUnix: 1}).Validate(); err != nil {
		return routingentity.RouteCircuit{}, false, err
	}
	if err := store.prepare(); err != nil {
		return routingentity.RouteCircuit{}, false, fmt.Errorf("prepare routing circuit store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-health.guard"))
	if err != nil {
		return routingentity.RouteCircuit{}, false, fmt.Errorf("lock routing circuit store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return routingentity.RouteCircuit{}, false, err
	}
	snapshot, err := store.readRouteCircuits()
	if err != nil {
		return routingentity.RouteCircuit{}, false, fmt.Errorf("read routing circuit store: %w", err)
	}
	for index, circuit := range snapshot.Circuits {
		if circuit.AccountID != accountID || circuit.Route != route {
			continue
		}
		updated, allowed, changed := circuit.ReserveProbe(healthScore, now)
		if !changed {
			return circuit, allowed, nil
		}
		if updated.UntilUnix == 0 {
			snapshot.Circuits = append(snapshot.Circuits[:index], snapshot.Circuits[index+1:]...)
		} else {
			snapshot.Circuits[index] = updated
		}
		if err := store.writeRouteCircuits(retainRouteCircuits(snapshot.Circuits, now)); err != nil {
			return routingentity.RouteCircuit{}, false, fmt.Errorf("reserve routing circuit probe: %w", err)
		}
		return updated, allowed, nil
	}
	return routingentity.RouteCircuit{}, true, nil
}

func (store *Store) OpenRouteCircuit(
	ctx context.Context,
	accountID, route string,
	healthScore uint8,
	now time.Time,
) (routingentity.RouteCircuit, bool, error) {
	if err := ctx.Err(); err != nil {
		return routingentity.RouteCircuit{}, false, err
	}
	if err := (routingentity.RouteCircuit{AccountID: accountID, Route: route, UntilUnix: 1}).Validate(); err != nil {
		return routingentity.RouteCircuit{}, false, err
	}
	if healthScore < routingentity.RouteCircuitHealthThreshold {
		return routingentity.RouteCircuit{}, false, nil
	}
	if err := store.prepare(); err != nil {
		return routingentity.RouteCircuit{}, false, fmt.Errorf("prepare routing circuit store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-health.guard"))
	if err != nil {
		return routingentity.RouteCircuit{}, false, fmt.Errorf("lock routing circuit store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return routingentity.RouteCircuit{}, false, err
	}
	snapshot, err := store.readRouteCircuits()
	if err != nil {
		return routingentity.RouteCircuit{}, false, fmt.Errorf("read routing circuit store: %w", err)
	}
	for index, current := range snapshot.Circuits {
		if current.AccountID == accountID && current.Route == route {
			circuit, opened := routingentity.OpenRouteCircuit(accountID, route, healthScore, &current, now)
			if !opened {
				return routingentity.RouteCircuit{}, false, nil
			}
			snapshot.Circuits[index] = circuit
			if err := store.writeRouteCircuits(retainRouteCircuits(snapshot.Circuits, now)); err != nil {
				return routingentity.RouteCircuit{}, false, fmt.Errorf("open routing circuit: %w", err)
			}
			return circuit, true, nil
		}
	}
	circuit, opened := routingentity.OpenRouteCircuit(accountID, route, healthScore, nil, now)
	if !opened {
		return routingentity.RouteCircuit{}, false, nil
	}
	snapshot.Circuits = append(snapshot.Circuits, circuit)
	if err := store.writeRouteCircuits(retainRouteCircuits(snapshot.Circuits, now)); err != nil {
		return routingentity.RouteCircuit{}, false, fmt.Errorf("open routing circuit: %w", err)
	}
	return circuit, true, nil
}

func (store *Store) ClearRouteCircuit(ctx context.Context, accountID, route string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := (routingentity.RouteCircuit{AccountID: accountID, Route: route, UntilUnix: 1}).Validate(); err != nil {
		return err
	}
	if err := store.prepare(); err != nil {
		return fmt.Errorf("prepare routing circuit store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-health.guard"))
	if err != nil {
		return fmt.Errorf("lock routing circuit store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := store.readRouteCircuits()
	if err != nil {
		return fmt.Errorf("read routing circuit store: %w", err)
	}
	remaining := snapshot.Circuits[:0]
	for _, circuit := range snapshot.Circuits {
		if circuit.AccountID != accountID || circuit.Route != route {
			remaining = append(remaining, circuit)
		}
	}
	if len(remaining) == len(snapshot.Circuits) {
		return nil
	}
	return store.writeRouteCircuits(remaining)
}
