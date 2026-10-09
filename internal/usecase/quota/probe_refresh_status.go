package quota

import (
	"context"
	"errors"
	"strings"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

func (status *Status) ensureProbeRefreshQueue() *ProbeRefreshQueue {
	status.probeQueueMu.Lock()
	defer status.probeQueueMu.Unlock()
	if status.probeQueue == nil {
		status.probeQueue = NewProbeRefreshQueue(context.Background(), ProbeRefreshOptions{})
	}
	return status.probeQueue
}

// ScheduleProbeRefresh refreshes one account without blocking the caller.
func (status *Status) ScheduleProbeRefresh(ctx context.Context, account accountentity.Account, baseURL string) error {
	if !account.Enabled {
		return nil
	}
	if strings.TrimSpace(account.ID) == "" {
		return errors.New("probe refresh account ID is required")
	}
	home := status.accounts.CodexHome(account.ID)
	return status.scheduleProbeRefresh(ctx, account.ID, home, baseURL)
}

func (status *Status) scheduleProbeRefresh(ctx context.Context, accountID, home, baseURL string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return errors.New("probe refresh account ID is required")
	}
	key := accountID + "\x00" + home + "\x00" + strings.TrimSpace(baseURL)
	return status.ensureProbeRefreshQueue().Schedule(ctx, key, func(runCtx context.Context) error {
		usage, err := status.fetchHomeUsage(runCtx, home, baseURL)
		if err != nil {
			return err
		}
		now := status.now()
		if strings.TrimSpace(baseURL) == "" {
			status.storeUsage(home, usage, now)
			status.storeUsageSnapshot(runCtx, accountID, usage, now)
		}
		return nil
	})
}

func (status *Status) ProbeRefreshBacklog() int {
	status.probeQueueMu.Lock()
	queue := status.probeQueue
	status.probeQueueMu.Unlock()
	return queue.Backlog()
}

func (status *Status) ProbeRefreshPressure() bool {
	status.probeQueueMu.Lock()
	queue := status.probeQueue
	status.probeQueueMu.Unlock()
	return queue.Pressure()
}

func (status *Status) ProbeRefreshRevision() uint64 {
	status.probeQueueMu.Lock()
	queue := status.probeQueue
	status.probeQueueMu.Unlock()
	return queue.Revision()
}

func (status *Status) WaitProbeRefresh(ctx context.Context, observed uint64) bool {
	status.probeQueueMu.Lock()
	queue := status.probeQueue
	status.probeQueueMu.Unlock()
	return queue.WaitProgress(ctx, observed)
}

func (status *Status) ShutdownProbeRefresh(ctx context.Context) error {
	status.probeQueueMu.Lock()
	queue := status.probeQueue
	status.probeQueueMu.Unlock()
	return queue.Shutdown(ctx)
}
