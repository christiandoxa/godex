package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/christiandoxa/godex/internal/config"
	"github.com/christiandoxa/godex/internal/delivery/cli"
	proxyhttp "github.com/christiandoxa/godex/internal/delivery/http/proxy"
	antigravitygateway "github.com/christiandoxa/godex/internal/gateway/antigravity"
	claudegateway "github.com/christiandoxa/godex/internal/gateway/claude"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	copilotgateway "github.com/christiandoxa/godex/internal/gateway/copilot"
	deepseekgateway "github.com/christiandoxa/godex/internal/gateway/deepseek"
	geminigateway "github.com/christiandoxa/godex/internal/gateway/gemini"
	githubgateway "github.com/christiandoxa/godex/internal/gateway/github"
	kirogateway "github.com/christiandoxa/godex/internal/gateway/kiro"
	"github.com/christiandoxa/godex/internal/gateway/openai"
	providerkeygateway "github.com/christiandoxa/godex/internal/gateway/providerkey"
	quotagateway "github.com/christiandoxa/godex/internal/gateway/quota"
	updategateway "github.com/christiandoxa/godex/internal/gateway/update"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/christiandoxa/godex/internal/repository/account"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	runtimerepo "github.com/christiandoxa/godex/internal/repository/runtime"
	sessionrepo "github.com/christiandoxa/godex/internal/repository/session"
	updaterepo "github.com/christiandoxa/godex/internal/repository/update"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
	pingusecase "github.com/christiandoxa/godex/internal/usecase/ping"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
	updateusecase "github.com/christiandoxa/godex/internal/usecase/update"
	"github.com/christiandoxa/godex/internal/version"
)

const errorPrefix = "godex:"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if arguments, ok := nativeAntigravityArguments(os.Args[1:]); ok {
		return runNativeAntigravity(ctx, arguments)
	}

	settings, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errorPrefix, err)
		return 1
	}

	store := account.NewFileStore(settings.Home)
	process := codex.NewCodexProcess(settings.CodexBin, codex.Terminal{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	antigravityProcess := antigravitygateway.NewProcess(settings.AgyBin, antigravitygateway.Terminal{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
	})
	login := authusecase.NewLogin(store, process)
	importer := authusecase.NewImportCurrent(store, process, settings.CurrentCodexHome)
	doctor := runtimeusecase.NewDoctor(store, process)
	quotaClient, err := openai.NewQuotaClient(settings.UpstreamURL, nil, process)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errorPrefix, err)
		return 1
	}
	quotaStatus := quotausecase.NewStatus(store, quotaClient)
	quotaStatus.SetModelProviderInspector(process)
	quotaStatus.SetExternalProvider("gemini", geminigateway.ProfileQuota{})
	autoRedeemer := quotausecase.NewAutoRedeemer(quotaClient)
	virtualQuota := quotagateway.NewVirtual(nil)
	quotaStatus.SetVirtual(virtualQuota)
	bindings := routingrepo.NewStore(settings.Home)
	copilotSource := copilotgateway.NewSource(nil)
	claudeSource := claudegateway.NewSource()
	kiroSource := kirogateway.NewSource()
	providerCatalogs := runtimerepo.NewProviderCatalogStore()
	activity := runtimeusecase.NewActivity(settings.Home, runtimerepo.NewLog(filepath.Join(settings.Home, "logs")), store, process)
	doctor.SetActivity(activity)
	doctor.SetQuota(quotaStatus)
	doctor.SetBundleStore(runtimerepo.NewDoctorBundleStore())
	factory := runtimeusecase.ProxyFactory(func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		router, err := newRuntimeRouter(config, runtimeRouterDependencies{
			process: process, copilotSource: copilotSource, claudeSource: claudeSource,
			kiroSource: kiroSource, providerCatalogs: providerCatalogs,
			bindings: bindings, autoRedeemer: autoRedeemer,
		})
		if err != nil {
			return nil, err
		}
		return proxyhttp.NewProxy(proxyhttp.Config{Router: router, Activity: activity})
	})
	runner := runtimeusecase.NewRunner(store, process, factory)
	runner.SetProviderCatalogStore(providerCatalogs)
	runner.SetQuotaPreflight(quotaStatus)
	runner.SetUpstreamURL(settings.UpstreamURL)
	runner.SetCurrentCodexHome(settings.CurrentCodexHome)
	runner.SetSharedCodexHome(settings.SharedCodexHome)
	runner.SetProviderCredentialResolver(providerkeygateway.NewSource())
	runner.SetAntigravityProcess(antigravityProcess)
	runner.SetAntigravitySessionLocker(codex.SessionLocker{})
	application := cli.New(login, importer, store, runner, doctor, quotaStatus, os.Stdout)
	application.SetErrorOutput(os.Stderr)
	profileStore := profilerepo.NewStore(settings.Home)
	doctor.SetImportJournalRepairer(profileStore)
	profiles := profileusecase.NewCatalog(profileStore, store, settings.CurrentCodexHome)
	doctor.SetSessionIndexRepairer(process)
	doctor.SetActiveCodexHomeResolver(profiles)
	doctor.SetSharedCodexHome(settings.SharedCodexHome)
	profiles.SetAuthInspector(process)
	profiles.SetClaudeSource(claudeSource)
	quotaStatus.SetExternalProvider("anthropic", claudeSource)
	profiles.SetKiroInspector(kiroSource)
	profiles.SetKiroSource(kiroSource)
	quotaStatus.SetExternalProvider("kiro", kiroSource)
	quotaStatus.SetExternalProvider("agy", virtualQuota)
	profiles.SetCopilotSource(copilotSource)
	quotaStatus.SetExternalProvider("copilot", copilotSource)
	application.SetProfiles(profiles)
	activity.SetProfiles(profiles)
	quotaStatus.SetProfiles(profiles)
	application.SetRedeemer(quotausecase.NewRedeemer(profiles, quotaClient))
	application.SetPing(pingusecase.NewOpenAI(profiles, process))
	releaseClient := githubgateway.NewEnvironmentReleaseClient("godex/"+version.Version, nil)
	application.SetUpdate(
		updateusecase.NewUpdater(releaseClient, updaterepo.NewStore(settings.Home), updategateway.NewInstaller(), version.Version),
		os.Stderr,
	)

	nativeAuth := authusecase.NewNative(store, process, antigravityProcess)
	nativeAuth.SetAntigravityCodexHome(settings.SharedCodexHome)
	nativeAuth.SetAntigravitySessionLocker(codex.SessionLocker{})
	application.SetNativeAuth(nativeAuth)
	application.SetActivity(activity)
	sessions := sessionusecase.NewCatalog(store, sessionrepo.NewReader(), runner)
	sessions.SetOwnerLookup(func(ctx context.Context, id string) (string, error) {
		return routingusecase.SessionOwner(ctx, bindings, id)
	})
	application.SetSessions(sessions)
	if err := application.Run(ctx, os.Args[1:]); err != nil {
		return exitCode(ctx, err)
	}
	return 0
}

type runtimeGateway interface {
	Execute(context.Context, proxyconfig.Request, proxyconfig.Account) (*proxyconfig.Response, error)
}

type runtimeRouterDependencies struct {
	process          *codex.CodexProcess
	copilotSource    *copilotgateway.Source
	claudeSource     *claudegateway.Source
	kiroSource       *kirogateway.Source
	providerCatalogs *runtimerepo.ProviderCatalogStore
	bindings         *routingrepo.Store
	autoRedeemer     *quotausecase.AutoRedeemer
}

func newRuntimeRouter(
	config proxyconfig.Config,
	dependencies runtimeRouterDependencies,
) (*routingusecase.Router, error) {
	gateway, err := newRuntimeGateway(
		config,
		dependencies.process,
		dependencies.copilotSource,
		dependencies.claudeSource,
		dependencies.kiroSource,
		dependencies.providerCatalogs,
	)
	if err != nil {
		return nil, err
	}
	return routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway, Accounts: runtimeAccountSource(config.Accounts, gateway),
		PreferredAccount: config.PreferredAccount, Bindings: dependencies.bindings,
		AutoRedeem: config.AutoRedeem, Redeemer: dependencies.autoRedeemer,
	})
}

func newRuntimeGateway(
	config proxyconfig.Config,
	process *codex.CodexProcess,
	copilotSource *copilotgateway.Source,
	claudeSource *claudegateway.Source,
	kiroSource *kirogateway.Source,
	providerCatalogs *runtimerepo.ProviderCatalogStore,
) (runtimeGateway, error) {
	switch config.Provider.Kind {
	case "", "openai":
		return openai.NewTransport(config.UpstreamURL, nil, process)
	case "copilot":
		return newCopilotRuntimeGateway(config, copilotSource, providerCatalogs)
	case "anthropic":
		return newAnthropicRuntimeGateway(config, claudeSource)
	case "deepseek":
		return deepseekgateway.NewRuntimePoolWithOptions(config.Provider.APIURL, config.ProviderCredentials, deepseekgateway.RequestOptions{
			StrictTools: config.Provider.StrictTools, WebSearchMode: config.Provider.WebSearchMode,
			BetaBaseURL: config.Provider.BetaBaseURL, SSELookaheadTimeout: config.Provider.SSELookaheadTimeout,
		}, nil)
	case "gemini":
		return geminigateway.NewRuntimePool(config.Provider.APIURL, config.ProviderCredentials, nil)
	case "kiro":
		return newKiroRuntimeGateway(config, kiroSource)
	default:
		return nil, fmt.Errorf("runtime provider %q is not implemented", config.Provider.Kind)
	}
}

func newKiroRuntimeGateway(config proxyconfig.Config, source *kirogateway.Source) (runtimeGateway, error) {
	if source == nil {
		return nil, errors.New("Kiro runtime source is not configured")
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	if config.Accounts == nil {
		return nil, errors.New("Kiro runtime account source is not configured")
	}
	accounts, err := config.Accounts(config.Context)
	if err != nil {
		return nil, err
	}
	return source.NewRuntimePool(config.Context, accounts)
}

func newAnthropicRuntimeGateway(
	config proxyconfig.Config,
	source *claudegateway.Source,
) (runtimeGateway, error) {
	if source == nil {
		return nil, errors.New("Anthropic runtime source is not configured")
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	if len(config.ProviderCredentials) > 0 {
		return claudegateway.NewRuntimeAPIKeyPool(config.Provider.APIURL, config.ProviderCredentials, nil)
	}
	if config.Accounts == nil {
		return nil, errors.New("Anthropic runtime account source is not configured")
	}
	accounts, err := config.Accounts(config.Context)
	if err != nil {
		return nil, err
	}
	return source.NewRuntimePool(config.Context, accounts)
}

func newCopilotRuntimeGateway(
	config proxyconfig.Config,
	source *copilotgateway.Source,
	catalogs *runtimerepo.ProviderCatalogStore,
) (runtimeGateway, error) {
	if source == nil {
		return nil, errors.New("Copilot runtime source is not configured")
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	if config.Accounts == nil {
		return nil, errors.New("Copilot runtime account source is not configured")
	}
	accounts, err := config.Accounts(config.Context)
	if err != nil {
		return nil, err
	}
	pool, err := source.NewRuntimePool(config.Context, accounts)
	if err != nil {
		return nil, err
	}
	if catalogs == nil || len(pool.ModelCatalog()) == 0 {
		return pool, nil
	}
	home, err := runtimeProviderHome(config)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := catalogs.WriteCopilotRuntime(home, pool.ModelCatalog()); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

type runtimeAccountAvailability interface {
	AvailableAccount(string) bool
}

func runtimeAccountSource(
	source func(context.Context) ([]proxyconfig.Account, error),
	gateway runtimeGateway,
) func(context.Context) ([]proxyconfig.Account, error) {
	available, ok := gateway.(runtimeAccountAvailability)
	if !ok || source == nil {
		return source
	}
	return func(ctx context.Context) ([]proxyconfig.Account, error) {
		accounts, err := source(ctx)
		if err != nil {
			return nil, err
		}
		filtered := make([]proxyconfig.Account, 0, len(accounts))
		for _, account := range accounts {
			if available.AvailableAccount(account.ID) {
				filtered = append(filtered, account)
			}
		}
		return filtered, nil
	}
}

func runtimeProviderHome(config proxyconfig.Config) (string, error) {
	if config.Accounts == nil {
		return "", errors.New("runtime provider account source is not configured")
	}
	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}
	accounts, err := config.Accounts(ctx)
	if err != nil {
		return "", err
	}
	for _, account := range accounts {
		if account.ID == config.PreferredAccount && account.Home != "" {
			return account.Home, nil
		}
	}
	if len(accounts) == 1 && accounts[0].Home != "" {
		return accounts[0].Home, nil
	}
	return "", errors.New("runtime provider profile home is unavailable")
}

func exitCode(ctx context.Context, err error) int {
	var childError *exec.ExitError
	if errors.As(err, &childError) {
		if code := childError.ExitCode(); code >= 0 {
			return code
		}
		if childError.ProcessState != nil {
			if status, ok := childError.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return 128 + int(status.Signal())
			}
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return 130
	}
	_, _ = fmt.Fprintln(os.Stderr, errorPrefix, err)
	return 1
}
