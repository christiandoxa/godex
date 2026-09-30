package quota

import (
	"context"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (status *Status) Availability(ctx context.Context, account accountentity.Account) (quotamodel.Availability, error) {
	if !account.Enabled {
		return quotamodel.Availability{}, nil
	}
	usage, err := status.usage.Fetch(ctx, status.accounts.CodexHome(account.ID))
	if err != nil {
		return quotamodel.Availability{}, err
	}
	now := status.now()
	ready := quotaState(quotamodel.Report{Enabled: true, Usage: usage}, now) != "exhausted"
	if ready {
		return quotamodel.Availability{Ready: true}, nil
	}
	retry := time.Time{}
	for _, window := range []*quotamodel.Window{usage.Primary, usage.Secondary} {
		if window == nil || window.ResetAt == nil || *window.ResetAt <= now.Unix() {
			continue
		}
		if window.UsedPercent != nil && *window.UsedPercent < 100 {
			continue
		}
		reset := time.Unix(*window.ResetAt, 0)
		if reset.After(retry) {
			retry = reset
		}
	}
	if retry.IsZero() {
		retry = now.Add(time.Minute)
	}
	return quotamodel.Availability{RetryAt: retry}, nil
}
