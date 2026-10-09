package quota

import (
	"context"
	"errors"
	"strconv"
	"strings"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
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
	return status.ScheduleProbeRefreshAtPolicy(ctx, account, baseURL, false)
}

// ScheduleProbeRefreshAtPolicy queues one account refresh using the same
// upstream transport policy as a request-time quota check. The queue keeps
// follow-up startup work off the launch path without silently re-enabling a
// proxy that the caller explicitly disabled.
func (status *Status) ScheduleProbeRefreshAtPolicy(
	ctx context.Context,
	account accountentity.Account,
	baseURL string,
	noProxy bool,
) error {
	if !account.Enabled {
		return nil
	}
	if strings.TrimSpace(account.ID) == "" {
		return errors.New("probe refresh account ID is required")
	}
	home := status.accounts.CodexHome(account.ID)
	return status.scheduleProbeRefresh(ctx, account.ID, home, baseURL, noProxy)
}

func (status *Status) scheduleProbeRefresh(ctx context.Context, accountID, home, baseURL string, noProxy bool) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return errors.New("probe refresh account ID is required")
	}
	key := accountID + "\x00" + home + "\x00" + strings.TrimSpace(baseURL) + "\x00" + strconv.FormatBool(noProxy)
	return status.ensureProbeRefreshQueue().Schedule(ctx, key, func(runCtx context.Context) error {
		usage, err := status.fetchHomeUsageAtPolicy(runCtx, home, baseURL, noProxy)
		if err != nil {
			return err
		}
		now := status.now()
		if strings.TrimSpace(baseURL) == "" {
			status.storeUsage(home, usage, now)
		}
		status.storeUsageSnapshot(runCtx, accountID, usage, now)
		return nil
	})
}

func (status *Status) fetchHomeUsageAtPolicy(
	ctx context.Context,
	home, baseURL string,
	noProxy bool,
) (quotamodel.Usage, error) {
	baseURL = strings.TrimSpace(baseURL)
	if !noProxy && baseURL == "" {
		return status.fetchHomeUsage(ctx, home, baseURL)
	}
	policy, ok := status.usage.(policyUsageGateway)
	if !ok {
		if baseURL != "" && !noProxy {
			override, supported := status.usage.(overrideUsageGateway)
			if !supported {
				return quotamodel.Usage{}, errors.New("quota base URL override is not supported")
			}
			return status.fetchProbe(ctx, func() (quotamodel.Usage, error) {
				return override.FetchAt(ctx, home, baseURL)
			})
		}
		return quotamodel.Usage{}, statusNoProxyUnsupported()
	}
	return status.fetchProbe(ctx, func() (quotamodel.Usage, error) {
		return policy.FetchAtPolicy(ctx, home, baseURL, noProxy)
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
