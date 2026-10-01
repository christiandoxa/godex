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
	claudegateway "github.com/christiandoxa/godex/internal/gateway/claude"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	copilotgateway "github.com/christiandoxa/godex/internal/gateway/copilot"
	githubgateway "github.com/christiandoxa/godex/internal/gateway/github"
	kirogateway "github.com/christiandoxa/godex/internal/gateway/kiro"
	"github.com/christiandoxa/godex/internal/gateway/openai"
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
	login := authusecase.NewLogin(store, process)
	importer := authusecase.NewImportCurrent(store, process, settings.CurrentCodexHome)
	doctor := runtimeusecase.NewDoctor(store, process)
	quotaClient, err := openai.NewQuotaClient(settings.UpstreamURL, nil, process)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errorPrefix, err)
		return 1
	}
	quotaStatus := quotausecase.NewStatus(store, quotaClient)
	bindings := routingrepo.NewStore(settings.Home)
	copilotSource := copilotgateway.NewSource(nil)
	providerCatalogs := runtimerepo.NewProviderCatalogStore()
	activity := runtimeusecase.NewActivity(settings.Home, runtimerepo.NewLog(filepath.Join(settings.Home, "logs")), store, process)
	doctor.SetActivity(activity)
	doctor.SetQuota(quotaStatus)
	doctor.SetBundleStore(runtimerepo.NewDoctorBundleStore())
	factory := runtimeusecase.ProxyFactory(func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		router, err := newRuntimeRouter(config, process, copilotSource, providerCatalogs, bindings)
		if err != nil {
			return nil, err
		}
		return proxyhttp.NewProxy(proxyhttp.Config{Router: router, Activity: activity})
	})
	runner := runtimeusecase.NewRunner(store, process, factory)
	runner.SetProviderCatalogStore(providerCatalogs)
	runner.SetQuotaPreflight(quotaStatus)
	runner.SetUpstreamURL(settings.UpstreamURL)
	application := cli.New(login, importer, store, runner, doctor, quotaStatus, os.Stdout)
	profileStore := profilerepo.NewStore(settings.Home)
	profiles := profileusecase.NewCatalog(profileStore, store, settings.CurrentCodexHome)
	profiles.SetAuthInspector(process)
	profiles.SetClaudeSource(claudegateway.NewSource())
	kiroSource := kirogateway.NewSource()
	profiles.SetKiroInspector(kiroSource)
	profiles.SetKiroSource(kiroSource)
	profiles.SetCopilotSource(copilotSource)
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

	application.SetNativeAuth(authusecase.NewNative(store, process))
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

func newRuntimeRouter(
	config proxyconfig.Config,
	process *codex.CodexProcess,
	copilotSource *copilotgateway.Source,
	providerCatalogs *runtimerepo.ProviderCatalogStore,
	bindings *routingrepo.Store,
) (*routingusecase.Router, error) {
	gateway, err := newRuntimeGateway(config, process, copilotSource, providerCatalogs)
	if err != nil {
		return nil, err
	}
	return routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway, Accounts: config.Accounts,
		PreferredAccount: config.PreferredAccount, Bindings: bindings,
	})
}

func newRuntimeGateway(
	config proxyconfig.Config,
	process *codex.CodexProcess,
	copilotSource *copilotgateway.Source,
	providerCatalogs *runtimerepo.ProviderCatalogStore,
) (runtimeGateway, error) {
	switch config.Provider.Kind {
	case "", "openai":
		return openai.NewTransport(config.UpstreamURL, nil, process)
	case "copilot":
		return newCopilotRuntimeGateway(config, copilotSource, providerCatalogs)
	default:
		return nil, fmt.Errorf("runtime provider %q is not implemented", config.Provider.Kind)
	}
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
	transport, err := source.NewRuntimeTransport(
		config.Context, config.Provider.Host, config.Provider.Login, config.Provider.APIURL,
	)
	if err != nil {
		return nil, err
	}
	if catalogs == nil || len(transport.ModelCatalog()) == 0 {
		return transport, nil
	}
	home, err := runtimeProviderHome(config)
	if err != nil {
		return nil, err
	}
	if _, err := catalogs.WriteCopilotRuntime(home, transport.ModelCatalog()); err != nil {
		return nil, err
	}
	return transport, nil
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
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return 130
	}
	_, _ = fmt.Fprintln(os.Stderr, errorPrefix, err)
	return 1
}
