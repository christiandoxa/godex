package routing

import (
	"context"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

// The capability is optional for existing injected routing repositories. Its
// state is strictly separate from generic rate/transport retry backoffs.
type authBackoffStore043714 interface {
	LoadAuthBackoffs(context.Context, time.Time) ([]routingentity.AuthBackoff, error)
	SetAuthBackoff(context.Context, routingentity.AuthBackoff, time.Time) error
	ClearAuthBackoff(context.Context, string, int64, time.Time) (bool, error)
}

func (router *Router) loadAuthBackoffs043714(ctx context.Context, now time.Time) error {
	store, ok := router.state.(authBackoffStore043714)
	if !ok {
		return nil
	}
	values, err := store.LoadAuthBackoffs(ctx, now)
	if err != nil {
		return err
	}
	for _, value := range values {
		// The inherited failure is known, but its prior bearer fingerprint
		// is *not* persisted. Recovery still requires a new HTTP receipt
		// bound to the current profile credentials, never a cached DTO.
		state := router.quarantine[value.AccountID]
		until := time.UnixMilli(value.UntilMillis)
		if state.until.Before(until) {
			state.until = until
		}
		state.authUntil = until
		state.authFailure = true
		state.authKnown = true
		state.authFailedAt = time.UnixMilli(value.FailedAtMillis)
		router.quarantine[value.AccountID] = state
	}
	return nil
}

func (router *Router) persistAuthBackoff043714(accountID string, state quarantineState) {
	if router == nil || router.state == nil || !router.persistAccountState(accountID) ||
		!router.persistenceWritesEnabled() || state.authFailedAt.IsZero() {
		return
	}
	store, ok := router.state.(authBackoffStore043714)
	if !ok {
		return
	}
	now := router.now()
	failure := routingentity.AuthBackoff{AccountID: accountID,
		FailedAtMillis: state.authFailedAt.UnixMilli(), UntilMillis: state.authUntil.UnixMilli()}
	if !failure.Active(now) {
		return
	}
	router.authBackoffMu.Lock()
	defer router.authBackoffMu.Unlock()
	// An optional write failure never clears the in-memory 401 penalty.
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	_ = store.SetAuthBackoff(ctx, failure, now)
}

// Caller holds the local authBackoffMu. A newer failure has its own epoch;
// conditional deletion cannot erase it after a concurrent successful probe.
func (router *Router) clearInheritedAuthBackoff043714(accountID string, state quarantineState, now time.Time) bool {
	store, ok := router.state.(authBackoffStore043714)
	if !ok || !router.persistAccountState(accountID) || !router.persistenceWritesEnabled() {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	removed, err := store.ClearAuthBackoff(ctx, accountID, state.authFailedAt.UnixMilli(), now)
	// No generic retry state is affected. If another process replaced the
	// failed auth generation, an older local receipt must never clear it.
	return err == nil && removed
}
