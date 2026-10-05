package routing

import (
	"context"
	"net/http"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
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
	now := router.now()
	seconds := int64((duration + time.Second - 1) / time.Second)
	backoff := routingentity.RetryBackoff{AccountID: accountID, UntilUnix: now.Unix() + seconds}

	router.retryBackoffMu.Lock()
	defer router.retryBackoffMu.Unlock()
	// Latest failure wins, including a shorter Retry-After than an older failure.
	router.replaceRetryQuarantine(accountID, time.Duration(seconds)*time.Second)
	if router.state != nil {
		_ = router.state.SetRetryBackoff(ctx, backoff, now)
	}
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
	if router.state != nil {
		_ = router.state.ClearRetryBackoff(ctx, accountID)
	}
}

func (router *Router) replaceRetryQuarantine(accountID string, duration time.Duration) {
	if duration <= 0 {
		duration = time.Second
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	current, exists := router.quarantine[accountID]
	if exists && current.authFailure && current.until.After(router.now()) {
		return
	}
	router.quarantine[accountID] = quarantineState{until: router.now().Add(duration)}
}

func retryBackoffCommitSuccess(response *proxymodel.Response, outcome responseOutcome) bool {
	return response != nil && !outcome.failed && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusBadRequest
}

func (router *Router) clearCommittedBackoffs(
	ctx context.Context,
	accountID string,
	request proxymodel.Request,
	response *proxymodel.Response,
	outcome responseOutcome,
) {
	if !retryBackoffCommitSuccess(response, outcome) {
		return
	}
	router.clearRetryBackoff(ctx, accountID)
	router.clearTransportBackoff(ctx, accountID, request)
}
