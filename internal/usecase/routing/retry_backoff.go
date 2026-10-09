package routing

import (
	"context"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

const defaultProfileRetryBackoff = 20 * time.Second

func (router *Router) persistRetryBackoff(ctx context.Context, accountID string, duration time.Duration) {
	if ctx.Err() != nil || accountID == "" {
		return
	}
	if duration <= 0 {
		duration = defaultProfileRetryBackoff
	}
	duration = min(duration, routingentity.MaxRetryBackoffDuration)

	router.retryBackoffMu.Lock()
	defer router.retryBackoffMu.Unlock()
	now := router.now()
	router.quarantineAccount(accountID, duration)
	if router.state == nil || !router.persistenceWritesEnabled() {
		return
	}
	seconds := int64((duration + time.Second - 1) / time.Second)
	backoff := routingentity.RetryBackoff{AccountID: accountID, UntilUnix: now.Unix() + seconds}
	// The in-memory quarantine remains effective if the optional durable write fails.
	_ = router.state.SetRetryBackoff(ctx, backoff, now)
}

func (router *Router) clearRetryBackoff(ctx context.Context, accountID string) {
	if ctx.Err() != nil || accountID == "" {
		return
	}
	router.retryBackoffMu.Lock()
	defer router.retryBackoffMu.Unlock()
	router.mu.Lock()
	if state, ok := router.quarantine[accountID]; ok && !state.authFailure {
		delete(router.quarantine, accountID)
	}
	router.mu.Unlock()
	if router.state != nil && router.persistAccountState(accountID) {
		_ = router.state.ClearRetryBackoff(ctx, accountID, router.now())
	}
}
