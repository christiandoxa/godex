package routing

import (
	"errors"
	"time"
)

const MaxRetryBackoffDuration = 24 * time.Hour

type RetryBackoff struct {
	AccountID string `json:"account_id"`
	UntilUnix int64  `json:"until_unix"`
}

func (backoff RetryBackoff) Validate() error {
	if !validAccountID(backoff.AccountID) || backoff.UntilUnix <= 0 {
		return errors.New("invalid routing retry backoff")
	}
	return nil
}

func (backoff RetryBackoff) Remaining(now time.Time) time.Duration {
	remaining := time.Unix(backoff.UntilUnix, 0).Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}
