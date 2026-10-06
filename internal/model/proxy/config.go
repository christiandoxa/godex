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
	ListenAddr          string
	PreferredAccount    string
	Provider            Provider
	ProviderCredentials []ProviderCredential
	AutoRedeem          bool
	SkipQuotaPreflight  bool
	Accounts            func(context.Context) ([]Account, error)
}
