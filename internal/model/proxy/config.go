package proxy

import (
	"context"
	"time"
)

type ProviderCredential struct {
	ID     string
	Secret string
}

type Account struct {
	ID            string
	Home          string
	Enabled       bool
	EligibleAfter time.Time
	RouteOrder    int
	Provider      Provider
}

type Config struct {
	Context             context.Context
	UpstreamURL         string
	PreferredAccount    string
	Provider            Provider
	ProviderCredentials []ProviderCredential
	Accounts            func(context.Context) ([]Account, error)
}
