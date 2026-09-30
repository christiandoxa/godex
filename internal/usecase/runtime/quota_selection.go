package runtime

import (
	"context"
	"errors"
	"fmt"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

func (runner *Runner) selectForLaunch(ctx context.Context, selector string) (accountentity.Account, map[string]bool, error) {
	if runner.quota == nil {
		selected, err := runner.accounts.SelectForLaunch(ctx, selector)
		return selected, nil, err
	}
	candidates, err := runner.accounts.LaunchCandidates(ctx, selector)
	if err != nil {
		return accountentity.Account{}, nil, err
	}
	exhausted := make(map[string]bool)
	var firstReady *accountentity.Account
	var firstUnknown *accountentity.Account
	for _, candidate := range candidates {
		ready, probeErr := runner.quota.Ready(ctx, candidate)
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
			exhausted[candidate.ID] = true
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
