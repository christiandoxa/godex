package routing

import (
	"context"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const maxFreshRecoveryWait = 30 * time.Second

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (router *Router) waitForFreshRecovery(
	ctx context.Context,
	delay time.Duration,
) error {
	return router.wait(ctx, min(delay, maxFreshRecoveryWait))
}

func (router *Router) freshRecoveryDelay(
	candidates []proxymodel.Account,
	retryable map[string]bool,
	requestID uint64,
	sweep int,
) (time.Duration, bool) {
	now := router.now()
	var earliest time.Duration
	ready := false
	for _, account := range candidates {
		if !retryable[account.ID] || !account.Enabled || account.EligibleAfter.After(now) {
			continue
		}
		if router.authFailureQuarantined(account.ID, now) {
			retryable[account.ID] = false
			continue
		}
		remaining := router.quarantineRemaining(account.ID, now)
		if remaining <= 0 {
			ready = true
			continue
		}
		if earliest == 0 || remaining < earliest {
			earliest = remaining
		}
	}
	if !hasRetryableFreshAccount(retryable) {
		return 0, false
	}
	if ready {
		return transientRecoveryBackoff(requestID, sweep), true
	}
	if earliest > 0 {
		return min(earliest, 30*time.Second), true
	}
	return transientRecoveryBackoff(requestID, sweep), true
}

func transientRecoveryBackoff(requestID uint64, sweep int) time.Duration {
	if sweep < 0 {
		sweep = 0
	}
	exponent := min(sweep, 5)
	base := 250 * time.Millisecond * time.Duration(1<<exponent)
	jitter := time.Duration((requestID+uint64(sweep))%251) * time.Millisecond
	return min(base+jitter, 30*time.Second)
}
