package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const gatewayOpenAIMountPath = "/backend-api/prodex"

type GatewayStartOptions struct {
	ListenAddr  string
	UpstreamURL string
}

type Gateway struct {
	proxy   Proxy
	release func() error
	once    sync.Once
}

func (gateway *Gateway) Endpoint() string {
	if gateway == nil || gateway.proxy == nil {
		return ""
	}
	return strings.TrimRight(gateway.proxy.Endpoint(), "/") + gatewayOpenAIMountPath
}

func (gateway *Gateway) Close(ctx context.Context) error {
	if gateway == nil {
		return nil
	}
	var result error
	gateway.once.Do(func() {
		if gateway.proxy != nil {
			result = gateway.proxy.Close(ctx)
		}
		if gateway.release != nil {
			result = errors.Join(result, gateway.release())
		}
	})
	return result
}

func (runner *Runner) StartGatewayCurrent(
	ctx context.Context,
	options GatewayStartOptions,
) (*Gateway, error) {
	account, err := runner.activeAccount(ctx, "")
	if err != nil {
		return nil, err
	}
	if !account.Enabled {
		return nil, errors.New("selected account is disabled")
	}
	return runner.StartGatewayAccount(ctx, account.ID, options)
}

func (runner *Runner) StartGatewayAccount(
	ctx context.Context,
	accountID string,
	options GatewayStartOptions,
) (*Gateway, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, errors.New("gateway account ID is required")
	}
	home, err := validateRuntimeHome(runner.accounts.CodexHome(accountID))
	if err != nil {
		return nil, err
	}
	release, err := runner.pinProfiles(ctx, []string{accountID})
	if err != nil {
		return nil, err
	}
	account := proxymodel.Account{ID: accountID, Home: home, Enabled: true}
	gateway, err := runner.startGateway(ctx, options, proxymodel.Provider{}, nil, []proxymodel.Account{account}, accountID)
	if err != nil {
		_ = release()
		return nil, err
	}
	gateway.release = release
	return gateway, nil
}

func (runner *Runner) StartGatewayProfile(
	ctx context.Context,
	codexHome string,
	provider proxymodel.Provider,
	options GatewayStartOptions,
) (*Gateway, error) {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return nil, err
	}
	idSeed := home
	if strings.TrimSpace(provider.Kind) != "" {
		idSeed = provider.Kind + ":" + home
	}
	accountID := profileRoutingID(idSeed)
	account := proxymodel.Account{
		ID: accountID, Home: home, Enabled: true, Provider: provider,
	}
	return runner.startGateway(ctx, options, provider, nil, []proxymodel.Account{account}, accountID)
}

func (runner *Runner) StartGatewayAPIKeys(
	ctx context.Context,
	codexHome string,
	provider proxymodel.Provider,
	apiKeys []string,
	options GatewayStartOptions,
) (*Gateway, error) {
	home, preferred, accounts, credentials, err := runner.providerAPIKeyPool(codexHome, provider, apiKeys)
	if err != nil {
		return nil, err
	}
	_ = home
	return runner.startGateway(ctx, options, provider, credentials, accounts, preferred)
}

func (runner *Runner) startGateway(
	ctx context.Context,
	options GatewayStartOptions,
	provider proxymodel.Provider,
	credentials []proxymodel.ProviderCredential,
	accounts []proxymodel.Account,
	preferredID string,
) (*Gateway, error) {
	if runner == nil || runner.newProxy == nil {
		return nil, errors.New("runtime proxy factory is not configured")
	}
	if len(accounts) == 0 || strings.TrimSpace(preferredID) == "" {
		return nil, errors.New("gateway profile pool is empty")
	}
	upstream := strings.TrimSpace(options.UpstreamURL)
	if upstream == "" {
		upstream = runner.upstream
	}
	config := runtimeProxyConfig(ctx, upstream, preferredID, provider, credentials, accounts, false)
	config.ListenAddr = strings.TrimSpace(options.ListenAddr)
	config.SkipQuotaPreflight = true
	proxy, err := runner.newProxy(config)
	if err != nil {
		return nil, err
	}
	if err := proxy.Start(); err != nil {
		_ = proxy.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return &Gateway{proxy: proxy}, nil
}
