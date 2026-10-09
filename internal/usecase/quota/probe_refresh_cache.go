package quota

import (
	"context"
	"time"
)

// Match Prodex's fast background refresh cadence near cache expiry.
const probeRefreshLead = 10 * time.Second

func (status *Status) scheduleCachedProbeRefresh(ctx context.Context, accountID, home string, now time.Time) {
	status.usageMu.Lock()
	snapshot, ok := status.usageCache[home]
	status.usageMu.Unlock()
	if !ok || now.Sub(snapshot.checkedAt) < usageCacheFreshness-probeRefreshLead {
		return
	}
	_ = status.scheduleProbeRefresh(ctx, accountID, home, "")
}
