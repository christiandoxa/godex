package quota

import (
	"context"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// Prodex 0.437.14: probe completion order must not replace the result of a
// newer-started observation. The serialized apply step covers both live cache
// and asynchronous/synchronous durable snapshot enqueue order; a late result
// is not an authenticated or quota recovery receipt.
func (status *Status) applySuccessfulUsageProbe043714(
	ctx context.Context, accountID, home string,
	usage quotamodel.Usage, now time.Time, generation uint64, liveCache bool,
) bool {
	if status == nil || ctx.Err() != nil {
		return false
	}
	status.probeApplyMu.Lock()
	defer status.probeApplyMu.Unlock()
	status.usageMu.Lock()
	if generation <= status.probeApplied[home] {
		status.usageMu.Unlock()
		return false
	}
	if status.probeApplied == nil {
		status.probeApplied = make(map[string]uint64)
	}
	status.probeApplied[home] = generation
	status.usageMu.Unlock()
	if liveCache {
		status.storeUsage(home, usage, now)
	}
	status.storeUsageSnapshot(ctx, accountID, usage, now)
	return true
}
