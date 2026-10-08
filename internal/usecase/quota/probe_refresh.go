package quota

import (
	"context"
	"sync"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const quotaProbeCapacity = 4

// probeRefreshGate bounds live quota requests without owning a worker pool.
// ponytail: fixed four-probe gate; tune only with measured provider pressure.
type probeRefreshGate struct {
	once  sync.Once
	slots chan struct{}
}

func (gate *probeRefreshGate) acquire(ctx context.Context) error {
	gate.once.Do(func() {
		gate.slots = make(chan struct{}, quotaProbeCapacity)
	})
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case gate.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (gate *probeRefreshGate) release() {
	<-gate.slots
}

func (status *Status) fetchProbe(ctx context.Context, fetch func() (quotamodel.Usage, error)) (quotamodel.Usage, error) {
	if err := status.probes.acquire(ctx); err != nil {
		return quotamodel.Usage{}, err
	}
	defer status.probes.release()
	return fetch()
}
