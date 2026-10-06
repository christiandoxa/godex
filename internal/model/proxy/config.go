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

type BrokerConfig struct {
	BrokerKey         string
	InstanceID        string
	AdminToken        string
	CurrentProfile    string
	StartedAt         int64
	IncludeCodeReview bool
	GodexVersion      string
	ExecutablePath    string
	ExecutableSHA256  string
	PersistenceRole   string
	ResolveProfile    func(context.Context, string) (string, error)
	OnActivated       func(context.Context, string) error
	LogRecovery       func(context.Context, string) error
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
	Broker              *BrokerConfig
	Accounts            func(context.Context) ([]Account, error)
}
