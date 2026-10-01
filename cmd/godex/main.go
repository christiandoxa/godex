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
	activity := runtimeusecase.NewActivity(settings.Home, runtimerepo.NewLog(filepath.Join(settings.Home, "logs")), store, process)
	doctor.SetActivity(activity)
	doctor.SetQuota(quotaStatus)
	doctor.SetBundleStore(runtimerepo.NewDoctorBundleStore())
	factory := runtimeusecase.ProxyFactory(func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		transport, err := openai.NewTransport(config.UpstreamURL, nil, process)
		if err != nil {
			return nil, err
		}
		router, err := routingusecase.NewRouter(routingusecase.Config{Gateway: transport, Accounts: config.Accounts, PreferredAccount: config.PreferredAccount, Bindings: bindings})
		if err != nil {
			return nil, err
		}
		return proxyhttp.NewProxy(proxyhttp.Config{Router: router, Activity: activity})
	})
	runner := runtimeusecase.NewRunner(store, process, factory)
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
	profiles.SetCopilotSource(copilotgateway.NewSource(nil))
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
