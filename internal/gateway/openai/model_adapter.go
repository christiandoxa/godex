package openai

import (
	"context"
	"errors"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

func NewProxyFromModel(config proxyconfig.Config) (*Proxy, error) {
	if config.Accounts == nil {
		return nil, errors.New("proxy account source is required")
	}
	return NewProxy(ProxyConfig{
		UpstreamURL:      config.UpstreamURL,
		PreferredAccount: config.PreferredAccount,
		Accounts: func(ctx context.Context) ([]RuntimeAccount, error) {
			profiles, err := config.Accounts(ctx)
			if err != nil {
				return nil, err
			}
			accounts := make([]RuntimeAccount, 0, len(profiles))
			for _, profile := range profiles {
				accounts = append(accounts, RuntimeAccount{
					ID: profile.ID, Home: profile.Home, Enabled: profile.Enabled,
				})
			}
			return accounts, nil
		},
	})
}
