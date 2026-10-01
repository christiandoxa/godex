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
	Context          context.Context
	UpstreamURL      string
	PreferredAccount string
	Provider         Provider
	Accounts         func(context.Context) ([]Account, error)
}
