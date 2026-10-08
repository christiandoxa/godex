package runtime

import (
	"context"
	"errors"
	"strings"

	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func localSessionCommand(command string) bool {
	return command == "delete" || command == "archive" || command == "unarchive"
}

func splitThreadSelector(value string) (string, string) {
	if selector, ok := strings.CutPrefix(value, "--thread="); ok {
		return "--thread=", selector
	}
	return "", value
}

func runNativeCommand(ctx context.Context, runner *runtimeusecase.Runner, selector string, arguments []string, index int) (bool, error) {
	if unsafeNativeCommand(arguments, index) {
		return true, errors.New("native command bypasses Godex routing; use godex exec or godex app-server without daemon/proxy")
	}
	switch arguments[index] {
	case "logout":
		return true, errors.New("use godex logout to safely mutate managed credentials")
	case "login":
		return true, runNativeLoginStatus(ctx, runner, selector, arguments, index)
	case "mcp", "features", "completion", "debug", "config", "delete", "archive", "unarchive", "version", "--version":
		return true, runner.RunLocal(ctx, selector, arguments)
	case "resume", "fork", "queue":
		return true, runner.RunCurrent(ctx, selector, arguments)
	default:
		return false, nil
	}
}

func runNativeLoginStatus(ctx context.Context, runner *runtimeusecase.Runner, selector string, arguments []string, index int) error {
	status := nextCommandWord(arguments, index+1)
	if status < 0 || arguments[status] != "status" {
		return errors.New("use godex login to register and safely update managed credentials")
	}
	return runner.RunLocal(ctx, selector, arguments)
}

func unsafeNativeCommand(args []string, command int) bool {
	nested := nextCommandWord(args, command+1)
	if nested < 0 {
		return false
	}
	switch args[command] {
	case "debug":
		return args[nested] == "app-server"
	case "app-server":
		return args[nested] == "daemon" || args[nested] == "proxy"
	}
	return false
}
