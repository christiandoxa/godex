package proxy

import "context"

type Account struct {
	ID      string
	Home    string
	Enabled bool
}

type Config struct {
	UpstreamURL      string
	PreferredAccount string
	Accounts         func(context.Context) ([]Account, error)
}
