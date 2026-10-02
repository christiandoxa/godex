package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type quotaProbe struct {
	ready   bool
	retryAt time.Time
	err     error
}

type candidateSnapshot struct {
	exhausted    map[string]time.Time
	firstReady   *accountentity.Account
	firstUnknown *accountentity.Account
}

func (runner *Runner) selectForLaunch(ctx context.Context, selector string) (accountentity.Account, map[string]time.Time, error) {
	if runner.quota == nil {
		selected, err := runner.accounts.SelectForLaunch(ctx, selector)
		return selected, nil, err
	}
	candidates, err := runner.accounts.LaunchCandidates(ctx, selector)
	if err != nil {
		return accountentity.Account{}, nil, err
	}
	snapshot, err := runner.probeCandidates(ctx, candidates)
	if err != nil {
		return accountentity.Account{}, nil, err
	}
	return runner.commitCandidate(ctx, candidates, snapshot)
}

func (runner *Runner) probeCandidates(ctx context.Context, candidates []accountentity.Account) (candidateSnapshot, error) {
	snapshot := candidateSnapshot{exhausted: make(map[string]time.Time)}
	for _, candidate := range candidates {
		probe := runner.probeCandidate(ctx, candidate)
		if ctx.Err() != nil {
			return candidateSnapshot{}, ctx.Err()
		}
		if probe.err != nil {
			if snapshot.firstUnknown == nil {
				copy := candidate
				snapshot.firstUnknown = &copy
			}
			continue
		}
		if !probe.ready {
			snapshot.exhausted[candidate.ID] = probe.retryAt
			continue
		}
		if snapshot.firstReady == nil {
			copy := candidate
			snapshot.firstReady = &copy
		}
	}
	return snapshot, nil
}

func (runner *Runner) probeCandidate(ctx context.Context, candidate accountentity.Account) quotaProbe {
	retryAt := time.Now().Add(time.Minute)
	if quota, ok := runner.quota.(interface {
		Availability(context.Context, accountentity.Account) (quotamodel.Availability, error)
	}); ok {
		availability, err := quota.Availability(ctx, candidate)
		if !availability.RetryAt.IsZero() {
			retryAt = availability.RetryAt
		}
		return quotaProbe{ready: availability.Ready, retryAt: retryAt, err: err}
	}
	ready, err := runner.quota.Ready(ctx, candidate)
	return quotaProbe{ready: ready, retryAt: retryAt, err: err}
}

func (runner *Runner) commitCandidate(ctx context.Context, candidates []accountentity.Account, snapshot candidateSnapshot) (accountentity.Account, map[string]time.Time, error) {
	if snapshot.firstReady != nil {
		selected, err := runner.accounts.SelectForLaunch(ctx, snapshot.firstReady.ID)
		return selected, snapshot.exhausted, err
	}
	if snapshot.firstUnknown != nil {
		selected, err := runner.accounts.SelectForLaunch(ctx, snapshot.firstUnknown.ID)
		return selected, snapshot.exhausted, err
	}
	if len(candidates) == 0 {
		return accountentity.Account{}, snapshot.exhausted, errors.New("no enabled account is available")
	}
	if runner.autoRedeem {
		selected, err := runner.accounts.SelectForLaunch(ctx, candidates[0].ID)
		return selected, snapshot.exhausted, err
	}
	if len(candidates) == 1 {
		return accountentity.Account{}, snapshot.exhausted, fmt.Errorf("account %q is currently quota exhausted", candidates[0].Name)
	}
	return accountentity.Account{}, snapshot.exhausted, errors.New("all enabled accounts are currently quota exhausted")
}
