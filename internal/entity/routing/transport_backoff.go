package routing

import (
	"encoding/hex"
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
	if len(backoff.AccountID) != 32 || backoff.UntilUnix <= 0 || !validTransportRoute(backoff.Route) {
		return errors.New("invalid routing transport backoff")
	}
	if _, err := hex.DecodeString(backoff.AccountID); err != nil {
		return errors.New("invalid routing transport backoff account")
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

func validTransportRoute(route string) bool {
	switch route {
	case "responses", "standard", "compact", "websocket":
		return true
	default:
		return false
	}
}
