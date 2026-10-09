package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const gatewayOpenAIMountPath = "/backend-api/godex"

type GatewayStartOptions struct {
	ListenAddr          string
	UpstreamURL         string
	SmartContextEnabled bool
	PresidioEnabled     bool
	PresidioRequired    bool
	Broker              *proxymodel.BrokerConfig
}

type Gateway struct {
	proxy   Proxy
	release func() error
	once    sync.Once
}

func (gateway *Gateway) SetPersistenceEnabled(enabled bool) {
	if gateway == nil || gateway.proxy == nil {
		return
	}
	if target, ok := gateway.proxy.(interface{ SetPersistenceEnabled(bool) }); ok {
		target.SetPersistenceEnabled(enabled)
	}
}

func (gateway *Gateway) SetPersistenceRole(role string) {
	if gateway == nil || gateway.proxy == nil {
		return
	}
	if target, ok := gateway.proxy.(interface{ SetBrokerPersistenceRole(string) }); ok {
		target.SetBrokerPersistenceRole(role)
	}
}

func (gateway *Gateway) RecordBrokerLog(line string) {
	if gateway == nil || gateway.proxy == nil {
		return
	}
	if target, ok := gateway.proxy.(interface{ RecordBrokerLog(string) }); ok {
		target.RecordBrokerLog(line)
	}
}

func (gateway *Gateway) ActiveRequests() int {
	if gateway == nil || gateway.proxy == nil {
		return 0
	}
	if active, ok := gateway.proxy.(interface{ ActiveRequests() int }); ok {
		return active.ActiveRequests()
	}
	return 0
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
	if err := runner.ensureAccountEnabled(ctx, accountID); err != nil {
		_ = release()
		return nil, err
	}
	account := proxymodel.Account{ID: accountID, Home: home, Enabled: true}
	gateway, err := runner.startGateway(ctx, options, proxymodel.Provider{}, nil, []proxymodel.Account{account}, accountID, true)
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
	return runner.startGateway(ctx, options, provider, nil, []proxymodel.Account{account}, accountID, true)
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
	return runner.startGateway(ctx, options, provider, credentials, accounts, preferred, true)
}

func (runner *Runner) StartBrokerGateway(
	ctx context.Context,
	preferredAccountID string,
	options GatewayStartOptions,
) (*Gateway, error) {
	preferredAccountID = strings.TrimSpace(preferredAccountID)
	if preferredAccountID == "" {
		return nil, errors.New("runtime broker preferred account is required")
	}
	if err := runner.prepareSharedAccountHomes(ctx); err != nil {
		return nil, err
	}
	profiles, err := runner.proxyAccounts(ctx, nil, "", preferredAccountID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		ids = append(ids, profile.ID)
	}
	release, err := runner.pinProfiles(ctx, ids)
	if err != nil {
		return nil, err
	}
	profiles, err = runner.pinnedAccounts(ctx, preferredAccountID, profiles)
	if err != nil {
		_ = release()
		return nil, err
	}
	gateway, err := runner.startGateway(
		ctx, options, proxymodel.Provider{}, nil, profiles, preferredAccountID, false,
	)
	if err != nil {
		_ = release()
		return nil, err
	}
	gateway.release = release
	return gateway, nil
}

func (runner *Runner) startGateway(
	ctx context.Context,
	options GatewayStartOptions,
	provider proxymodel.Provider,
	credentials []proxymodel.ProviderCredential,
	accounts []proxymodel.Account,
	preferredID string,
	skipQuotaPreflight bool,
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
	config.SkipQuotaPreflight = skipQuotaPreflight
	config.SmartContextEnabled = options.SmartContextEnabled
	if options.PresidioEnabled {
		if runner.presidioResolver == nil {
			return nil, errors.New("Presidio runtime resolver is not configured")
		}
		presidioConfig, err := runner.presidioResolver(ctx, options.PresidioRequired)
		if err != nil {
			return nil, err
		}
		config.Presidio = presidioConfig
	}
	config.Broker = options.Broker
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
