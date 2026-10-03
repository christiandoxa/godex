package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/christiandoxa/godex/internal/config"
	"github.com/christiandoxa/godex/internal/delivery/cli"
	runtimecli "github.com/christiandoxa/godex/internal/delivery/cli/runtime"
	antigravitygateway "github.com/christiandoxa/godex/internal/gateway/antigravity"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func nativeAntigravityArguments(arguments []string) ([]string, bool) {
	if len(arguments) > 0 && arguments[0] == "run" {
		arguments = arguments[1:]
	} else if len(arguments) > 0 && cli.IsExplicitGodexCommand(arguments[0]) {
		return nil, false
	}
	arguments = runtimecli.NormalizeNativeAntigravityArguments(arguments)
	if !runtimecli.UsesNativeAntigravity(arguments) {
		return nil, false
	}
	return arguments, true
}

func runNativeAntigravity(ctx context.Context, arguments []string) int {
	settings, err := config.LoadAntigravity()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errorPrefix, err)
		return 1
	}
	process := antigravitygateway.NewProcess(settings.AgyBin, antigravitygateway.Terminal{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
	})
	runner := runtimeusecase.NewRunner(nil, nil, nil)
	runner.SetAntigravityProcess(process)
	runner.SetAntigravityCodexHome(settings.SharedCodexHome)
	runner.SetAntigravitySessionLocker(codex.SessionLocker{})
	if err := runtimecli.Run(ctx, runner, nil, arguments, os.Stdout); err != nil {
		return antigravityExitCode(ctx, err, os.Stderr)
	}
	return 0
}

func antigravityExitCode(ctx context.Context, err error, stderr io.Writer) int {
	var childError *exec.ExitError
	if errors.As(err, &childError) {
		_, _ = fmt.Fprintln(stderr, "Error: Antigravity CLI exited unsuccessfully")
	}
	return exitCode(ctx, err)
}
