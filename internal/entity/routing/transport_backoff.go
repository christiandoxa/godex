package routing

import (
	"errors"
	"time"
)

const (
	InitialTransportBackoffDuration = 15 * time.Second
	MaxTransportBackoffDuration     = 120 * time.Second
)

type TransportBackoff struct {
	AccountID string `json:"account_id"`
	Route     string `json:"route"`
	UntilUnix int64  `json:"until_unix"`
}

func (backoff TransportBackoff) Validate() error {
	if !validAccountID(backoff.AccountID) || !validRoute(backoff.Route) || backoff.UntilUnix <= 0 {
		return errors.New("invalid routing transport backoff")
	}
	return nil
}

func (backoff TransportBackoff) Remaining(now time.Time) time.Duration {
	remaining := time.Unix(backoff.UntilUnix, 0).Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}
