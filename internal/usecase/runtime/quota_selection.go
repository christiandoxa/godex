package runtime

import (
	"context"
	"errors"
	"fmt"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

func (runner *Runner) selectForLaunch(ctx context.Context, selector string) (accountentity.Account, map[string]time.Time, error) {
	if runner.quota == nil {
		selected, err := runner.accounts.SelectForLaunch(ctx, selector)
		return selected, nil, err
	}
	candidates, err := runner.accounts.LaunchCandidates(ctx, selector)
	if err != nil {
		return accountentity.Account{}, nil, err
	}
	exhausted := make(map[string]time.Time)
	var firstReady *accountentity.Account
	var firstUnknown *accountentity.Account
	for _, candidate := range candidates {
		ready, probeErr := false, error(nil)
		retryAt := time.Now().Add(time.Minute)
		if quota, ok := runner.quota.(interface {
			Availability(context.Context, accountentity.Account) (quotamodel.Availability, error)
		}); ok {
			availability, err := quota.Availability(ctx, candidate)
			ready, probeErr = availability.Ready, err
			if !availability.RetryAt.IsZero() {
				retryAt = availability.RetryAt
			}
		} else {
			ready, probeErr = runner.quota.Ready(ctx, candidate)
		}
		if ctx.Err() != nil {
			return accountentity.Account{}, nil, ctx.Err()
		}
		if probeErr != nil {
			if firstUnknown == nil {
				copy := candidate
				firstUnknown = &copy
			}
			continue
		}
		if !ready {
			exhausted[candidate.ID] = retryAt
			continue
		}
		if firstReady == nil {
			copy := candidate
			firstReady = &copy
		}
	}
	if firstReady != nil {
		selected, err := runner.accounts.SelectForLaunch(ctx, firstReady.ID)
		return selected, exhausted, err
	}
	if firstUnknown != nil {
		selected, err := runner.accounts.SelectForLaunch(ctx, firstUnknown.ID)
		return selected, exhausted, err
	}
	if len(candidates) == 1 {
		return accountentity.Account{}, exhausted, fmt.Errorf("account %q is currently quota exhausted", candidates[0].Name)
	}
	if len(candidates) == 0 {
		return accountentity.Account{}, exhausted, errors.New("no enabled account is available")
	}
	return accountentity.Account{}, exhausted, errors.New("all enabled accounts are currently quota exhausted")
}
