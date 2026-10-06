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
	return runner.selectForLaunchWithPolicy(ctx, selector, false)
}

func (runner *Runner) selectForLaunchWithPolicy(ctx context.Context, selector string, noProxy bool) (accountentity.Account, map[string]time.Time, error) {
	return runner.selectForLaunchWithRuntimePolicy(ctx, selector, "", noProxy, false, runner.autoRedeem)
}

func (runner *Runner) selectForLaunchWithRuntimePolicy(
	ctx context.Context,
	selector string,
	baseURL string,
	noProxy bool,
	skipQuota bool,
	autoRedeem bool,
) (accountentity.Account, map[string]time.Time, error) {
	if skipQuota || runner.quota == nil {
		selected, err := runner.accounts.SelectForLaunch(ctx, selector)
		return selected, nil, err
	}
	candidates, err := runner.accounts.LaunchCandidates(ctx, selector)
	if err != nil {
		return accountentity.Account{}, nil, err
	}
	snapshot, err := runner.probeCandidatesWithRuntimePolicy(ctx, candidates, baseURL, noProxy)
	if err != nil {
		return accountentity.Account{}, nil, err
	}
	return runner.commitCandidateWithAutoRedeem(ctx, candidates, snapshot, autoRedeem)
}

func (runner *Runner) probeCandidates(ctx context.Context, candidates []accountentity.Account) (candidateSnapshot, error) {
	return runner.probeCandidatesWithPolicy(ctx, candidates, false)
}

func (runner *Runner) probeCandidatesWithPolicy(ctx context.Context, candidates []accountentity.Account, noProxy bool) (candidateSnapshot, error) {
	return runner.probeCandidatesWithRuntimePolicy(ctx, candidates, "", noProxy)
}

func (runner *Runner) probeCandidatesWithRuntimePolicy(
	ctx context.Context,
	candidates []accountentity.Account,
	baseURL string,
	noProxy bool,
) (candidateSnapshot, error) {
	snapshot := candidateSnapshot{exhausted: make(map[string]time.Time)}
	for _, candidate := range candidates {
		probe := runner.probeCandidateWithRuntimePolicy(ctx, candidate, baseURL, noProxy)
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
	return runner.probeCandidateWithPolicy(ctx, candidate, false)
}

func (runner *Runner) probeCandidateWithPolicy(ctx context.Context, candidate accountentity.Account, noProxy bool) quotaProbe {
	return runner.probeCandidateWithRuntimePolicy(ctx, candidate, "", noProxy)
}

func (runner *Runner) probeCandidateWithRuntimePolicy(
	ctx context.Context,
	candidate accountentity.Account,
	baseURL string,
	noProxy bool,
) quotaProbe {
	retryAt := time.Now().Add(time.Minute)
	if baseURL != "" || noProxy {
		if quota, ok := runner.quota.(interface {
			AvailabilityAtPolicy(context.Context, accountentity.Account, string, bool) (quotamodel.Availability, error)
		}); ok {
			availability, err := quota.AvailabilityAtPolicy(ctx, candidate, baseURL, noProxy)
			if !availability.RetryAt.IsZero() {
				retryAt = availability.RetryAt
			}
			return quotaProbe{ready: availability.Ready, retryAt: retryAt, err: err}
		}
		if baseURL != "" {
			return quotaProbe{retryAt: retryAt, err: errors.New("quota base URL override is not supported")}
		}
		if quota, ok := runner.quota.(interface {
			AvailabilityWithPolicy(context.Context, accountentity.Account, bool) (quotamodel.Availability, error)
		}); ok {
			availability, err := quota.AvailabilityWithPolicy(ctx, candidate, true)
			if !availability.RetryAt.IsZero() {
				retryAt = availability.RetryAt
			}
			return quotaProbe{ready: availability.Ready, retryAt: retryAt, err: err}
		}
	}
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
	return runner.commitCandidateWithAutoRedeem(ctx, candidates, snapshot, runner.autoRedeem)
}

func (runner *Runner) commitCandidateWithAutoRedeem(
	ctx context.Context,
	candidates []accountentity.Account,
	snapshot candidateSnapshot,
	autoRedeem bool,
) (accountentity.Account, map[string]time.Time, error) {
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
	if autoRedeem {
		selected, err := runner.accounts.SelectForLaunch(ctx, candidates[0].ID)
		return selected, snapshot.exhausted, err
	}
	if len(candidates) == 1 {
		return accountentity.Account{}, snapshot.exhausted, fmt.Errorf("account %q is currently quota exhausted", candidates[0].Name)
	}
	return accountentity.Account{}, snapshot.exhausted, errors.New("all enabled accounts are currently quota exhausted")
}
