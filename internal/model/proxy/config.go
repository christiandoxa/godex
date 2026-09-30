package proxy

import (
	"context"
	"time"
)

type Account struct {
	ID            string
	Home          string
	Enabled       bool
	EligibleAfter time.Time
}

type Config struct {
	UpstreamURL      string
	PreferredAccount string
	Accounts         func(context.Context) ([]Account, error)
}
