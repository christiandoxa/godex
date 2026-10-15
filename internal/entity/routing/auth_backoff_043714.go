package routing

import (
	"errors"
	"time"
)

// AuthBackoff is restart-safe 401/403 failure metadata. It deliberately does
// not persist OAuth tokens, bearer digests, or unrelated transport cooldowns.
type AuthBackoff struct {
	AccountID      string `json:"account_id"`
	FailedAtMillis int64  `json:"failed_at_ms"`
	UntilMillis    int64  `json:"until_ms"`
}

const MaxAuthBackoffDuration = 2 * time.Hour

func (backoff AuthBackoff) Validate() error {
	if !validAccountID(backoff.AccountID) || backoff.FailedAtMillis <= 0 ||
		backoff.UntilMillis <= backoff.FailedAtMillis ||
		backoff.UntilMillis-backoff.FailedAtMillis > MaxAuthBackoffDuration.Milliseconds() {
		return errors.New("invalid or oversized auth backoff")
	}
	return nil
}

func (backoff AuthBackoff) Active(now time.Time) bool {
	return backoff.UntilMillis > now.UnixMilli()
}
