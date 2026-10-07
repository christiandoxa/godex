package quota

import (
	"context"
	"math"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (status *Status) StatusSummary(ctx context.Context) (quotamodel.StatusSummary, error) {
	var summary quotamodel.StatusSummary
	if status == nil || status.profiles == nil {
		return summary, nil
	}
	targets, err := status.profiles.QuotaTargets(ctx)
	if err != nil {
		return summary, err
	}
	now := status.now()
	for _, target := range targets {
		if !target.Compatible {
			continue
		}
		summary.CompatibleProfiles++
		usage, ok := status.localStatusUsage(ctx, target.Name, target.AccountID, target.CodexHome, now)
		if !ok {
			summary.UnavailableProfiles++
			continue
		}
		fivePresent := accumulateStatusWindow(&summary.FiveHour, usage.Primary)
		weeklyPresent := accumulateStatusWindow(&summary.Weekly, usage.Secondary)
		if !fivePresent && !weeklyPresent {
			summary.UnavailableProfiles++
		}
	}
	return summary, nil
}

func (status *Status) localStatusUsage(
	ctx context.Context,
	profileName, accountID, home string,
	now time.Time,
) (quotamodel.Usage, bool) {
	if usage, ok := status.cachedLiveUsage(home, now); ok {
		return usage, true
	}
	for _, key := range statusSnapshotKeys(profileName, accountID) {
		if snapshot, ok := status.cachedPersistedSnapshot(ctx, key, now); ok {
			return usageFromSnapshot(snapshot), true
		}
	}
	return quotamodel.Usage{}, false
}

func statusSnapshotKeys(profileName, accountID string) []string {
	result := make([]string, 0, 2)
	for _, value := range []string{profileName, accountID} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		duplicate := false
		for _, current := range result {
			if current == value {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, value)
		}
	}
	return result
}

func accumulateStatusWindow(summary *quotamodel.StatusWindowSummary, window *quotamodel.Window) bool {
	if summary == nil || window == nil || window.UsedPercent == nil {
		return false
	}
	remaining := max(int64(0), min(int64(100), 100-*window.UsedPercent))
	summary.Profiles++
	summary.TotalRemaining += remaining
	if window.ResetAt != nil && *window.ResetAt != math.MaxInt64 {
		if summary.EarliestResetAt == 0 || *window.ResetAt < summary.EarliestResetAt {
			summary.EarliestResetAt = *window.ResetAt
		}
	}
	return true
}
