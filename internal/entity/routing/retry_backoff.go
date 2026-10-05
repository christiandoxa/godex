package routing

import (
	"encoding/hex"
	"errors"
	"time"
)

const MaxRetryBackoffDuration = 24 * time.Hour

type RetryBackoff struct {
	AccountID string `json:"account_id"`
	UntilUnix int64  `json:"until_unix"`
}

func (backoff RetryBackoff) Validate() error {
	if len(backoff.AccountID) != 32 || backoff.UntilUnix <= 0 {
		return errors.New("invalid routing retry backoff")
	}
	if _, err := hex.DecodeString(backoff.AccountID); err != nil {
		return errors.New("invalid routing retry backoff account")
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
