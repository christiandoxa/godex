package runtime

import (
	"context"
	"errors"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func runStandaloneProfileWithOptions(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	home string,
	provider proxymodel.Provider,
	arguments []string,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) error {
	runModel := func(args []string) error {
		if provider.Kind == "" {
			return runner.RunProfileWithOptions(ctx, home, args, launchOptions)
		}
		return runner.RunProviderProfileWithOptions(ctx, home, provider, args, launchOptions)
	}
	if index, args := sessionArgument(arguments); index >= 0 {
		_ = index
		return runModel(args)
	}
	if index := nativeCommandIndex(arguments); index >= 0 {
		if unsafeNativeCommand(arguments, index) {
			return errors.New("native command bypasses Godex routing; use godex exec or godex app-server without daemon/proxy")
		}
		switch arguments[index] {
		case "logout":
			return errors.New("use godex logout to safely mutate managed credentials")
		case "login":
			return runStandaloneLoginStatus(ctx, runner, home, arguments, index)
		case "mcp", "features", "completion", "debug", "config", "delete", "archive", "unarchive", "version", "--version":
			return runner.RunHome(ctx, home, arguments)
		case "resume", "fork", "queue":
			return runModel(arguments)
		}
	}
	return runModel(arguments)
}

func runStandaloneLoginStatus(ctx context.Context, runner *runtimeusecase.Runner, home string, arguments []string, index int) error {
	status := nextCommandWord(arguments, index+1)
	if status < 0 || arguments[status] != "status" {
		return errors.New("use godex login to register and safely update managed credentials")
	}
	return runner.RunHome(ctx, home, arguments)
}
