package routing

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

// InspectRouteCircuits returns the persisted diagnostic view of routing
// circuits without softening, clearing, or reserving a probe. The normal
// LoadRouteCircuits method remains responsible for restart reconciliation.
// A circuit whose cooldown has elapsed but whose health is still recent is
// observable as half-open by the runtime doctor.
func (store *Store) InspectRouteCircuits(
	ctx context.Context, now time.Time,
) ([]routingentity.RouteCircuit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, fmt.Errorf("prepare routing circuit inspection: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-health.guard"))
	if err != nil {
		return nil, fmt.Errorf("lock routing circuit inspection: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot, err := store.readRouteCircuits()
	if err != nil {
		return nil, fmt.Errorf("read routing circuit inspection: %w", err)
	}
	cutoff := now.Add(-routingentity.RouteHealthRetention).Unix()
	result := make([]routingentity.RouteCircuit, 0, len(snapshot.Circuits))
	for _, circuit := range snapshot.Circuits {
		if circuit.StageUpdatedUnix > cutoff {
			result = append(result, circuit)
		}
	}
	return result, nil
}
