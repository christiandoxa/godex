package runtime

import (
	"context"
	"errors"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

type launchAccounts interface {
	LaunchCandidates(context.Context, string) ([]accountentity.Account, error)
	SelectForLaunch(context.Context, string) (accountentity.Account, error)
	List(context.Context) ([]accountentity.Account, error)
	CodexHome(string) string
}

type codexProcess interface {
	Run(context.Context, string, []string) error
}

type proxyCodex interface {
	codexProcess
	CheckProxySupport(context.Context) error
	RunThroughProxy(context.Context, string, string, []string) error
}

type providerProxyCodex interface {
	RunThroughProxyProvider(context.Context, string, string, []string, string) error
}

type quotaPreflight interface {
	Ready(context.Context, accountentity.Account) (bool, error)
}

type providerCredentialResolver interface {
	APIKeys(string, string) ([]string, error)
}

type Runner struct {
	accounts    launchAccounts
	process     codexProcess
	newProxy    ProxyFactory
	quota       quotaPreflight
	catalog     providerCatalogStore
	upstream    string
	currentHome string
	credentials providerCredentialResolver
}

func NewRunner(accounts launchAccounts, process codexProcess, newProxy ProxyFactory) *Runner {
	return &Runner{accounts: accounts, process: process, newProxy: newProxy}
}

func (runner *Runner) SetQuotaPreflight(preflight quotaPreflight) {
	runner.quota = preflight
}

func (runner *Runner) SetProviderCatalogStore(store providerCatalogStore) {
	runner.catalog = store
}

func (runner *Runner) SetUpstreamURL(upstream string) {
	if upstream != "" {
		runner.upstream = upstream
	}
}

func (runner *Runner) SetCurrentCodexHome(home string) {
	runner.currentHome = home
}

func (runner *Runner) SetProviderCredentialResolver(resolver providerCredentialResolver) {
	runner.credentials = resolver
}

func (runner *Runner) ProviderAPIKeys(provider, explicit string) ([]string, error) {
	if runner.credentials == nil {
		return nil, errors.New("runtime provider credential resolver is not configured")
	}
	return runner.credentials.APIKeys(provider, explicit)
}

func (runner *Runner) CurrentCodexHome() string {
	return runner.currentHome
}

func (runner *Runner) Run(ctx context.Context, selector string, arguments []string) (runErr error) {
	selected, exhausted, err := runner.selectForLaunch(ctx, selector)
	if err != nil {
		return err
	}
	profiles, err := runner.proxyAccounts(ctx, exhausted, selector, selected.ID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		ids = append(ids, profile.ID)
	}
	release, err := runner.pinProfiles(ctx, ids)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	return runner.launch(ctx, selected.ID, selected.ID, profiles, arguments)
}

func (runner *Runner) launch(ctx context.Context, homeID, preferredID string, profiles []proxyconfig.Account, arguments []string) (runErr error) {
	profiles, err := runner.pinnedAccounts(ctx, preferredID, profiles)
	if err != nil {
		return err
	}
	return runner.launchHome(ctx, runner.accounts.CodexHome(homeID), preferredID, proxyconfig.Provider{}, nil, profiles, arguments)
}

func (runner *Runner) launchHome(
	ctx context.Context,
	home, preferredID string,
	provider proxyconfig.Provider,
	credentials []proxyconfig.ProviderCredential,
	profiles []proxyconfig.Account,
	arguments []string,
) error {
	proxyRunner, ok := runner.process.(proxyCodex)
	if !ok {
		return runner.process.Run(ctx, home, arguments)
	}
	if runner.newProxy == nil {
		return errors.New("runtime proxy factory is not configured")
	}
	if err := proxyRunner.CheckProxySupport(ctx); err != nil {
		return err
	}
	provider, profiles, err := runner.prepareProviderLaunch(home, provider, profiles)
	if err != nil {
		return err
	}
	proxy, err := runner.newProxy(runtimeProxyConfig(ctx, runner.upstream, preferredID, provider, credentials, profiles))
	if err != nil {
		return err
	}
	runtimeArguments, err := prepareProviderRuntimeArguments(runner.catalog, home, provider, arguments)
	if err != nil {
		return err
	}
	if err := proxy.Start(); err != nil {
		return err
	}
	runErr := runner.runThroughRuntimeProxy(ctx, proxyRunner, home, proxy.Endpoint(), runtimeArguments, provider.Kind)
	return closeRuntimeProxy(ctx, proxy, runErr)
}

func (runner *Runner) prepareProviderLaunch(
	home string,
	provider proxyconfig.Provider,
	profiles []proxyconfig.Account,
) (proxyconfig.Provider, []proxyconfig.Account, error) {
	if provider.Kind != "deepseek" {
		return provider, profiles, nil
	}
	if err := applyDeepSeekRuntimeSettings(home, &provider); err != nil {
		return proxyconfig.Provider{}, nil, err
	}
	result := append([]proxyconfig.Account(nil), profiles...)
	for index := range result {
		result[index].Provider.StrictTools = provider.StrictTools
		result[index].Provider.WebSearchMode = provider.WebSearchMode
		result[index].Provider.BetaBaseURL = provider.BetaBaseURL
	}
	return provider, result, nil
}

func runtimeProxyConfig(
	ctx context.Context,
	upstream, preferredID string,
	provider proxyconfig.Provider,
	credentials []proxyconfig.ProviderCredential,
	profiles []proxyconfig.Account,
) proxyconfig.Config {
	return proxyconfig.Config{
		Context:             ctx,
		UpstreamURL:         upstream,
		PreferredAccount:    preferredID,
		Provider:            provider,
		ProviderCredentials: append([]proxyconfig.ProviderCredential(nil), credentials...),
		Accounts: func(ctx context.Context) ([]proxyconfig.Account, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return append([]proxyconfig.Account(nil), profiles...), nil
		},
	}
}

func (runner *Runner) runThroughRuntimeProxy(
	ctx context.Context,
	proxyRunner proxyCodex,
	home, endpoint string,
	arguments []string,
	providerKind string,
) error {
	if providerKind != "" {
		if isolated, ok := runner.process.(providerProxyCodex); ok {
			return isolated.RunThroughProxyProvider(ctx, home, endpoint, arguments, providerKind)
		}
	}
	return proxyRunner.RunThroughProxy(ctx, home, endpoint, arguments)
}

func closeRuntimeProxy(ctx context.Context, proxy Proxy, runErr error) error {
	closeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if closeErr := proxy.Close(closeContext); runErr == nil {
		return closeErr
	}
	return runErr
}

func (runner *Runner) proxyAccounts(ctx context.Context, exhausted map[string]time.Time, selector, selectedID string) ([]proxyconfig.Account, error) {
	accounts, err := runner.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]proxyconfig.Account, 0, len(accounts))
	for _, account := range accounts {
		if selector != "" && account.ID != selectedID {
			continue
		}
		profiles = append(profiles, proxyconfig.Account{
			ID:            account.ID,
			Home:          runner.accounts.CodexHome(account.ID),
			Enabled:       account.Enabled,
			EligibleAfter: exhausted[account.ID],
		})
	}
	return profiles, nil
}
