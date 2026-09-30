package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/christiandoxa/godex/internal/config"
	"github.com/christiandoxa/godex/internal/delivery/cli"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	"github.com/christiandoxa/godex/internal/gateway/openai"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/christiandoxa/godex/internal/repository/account"
	sessionrepo "github.com/christiandoxa/godex/internal/repository/session"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	settings, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "godex:", err)
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
	quotaClient, err := openai.NewQuotaClient(settings.UpstreamURL, nil)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "godex:", err)
		return 1
	}
	quotaStatus := quotausecase.NewStatus(store, quotaClient)
	factory := runtimeusecase.ProxyFactory(func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		return openai.NewProxyFromModel(config)
	})
	runner := runtimeusecase.NewRunner(store, process, factory)
	runner.SetQuotaPreflight(quotaStatus)
	runner.SetUpstreamURL(settings.UpstreamURL)
	application := cli.New(login, importer, store, runner, doctor, quotaStatus, os.Stdout)

	application.SetSessions(sessionusecase.NewCatalog(store, sessionrepo.NewReader(), runner))
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
	_, _ = fmt.Fprintln(os.Stderr, "godex:", err)
	return 1
}
