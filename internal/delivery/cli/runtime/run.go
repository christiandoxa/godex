package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

func Launch(ctx context.Context, runner *runtimeusecase.Runner) error {
	return runner.Run(ctx, "", nil)
}

func Run(ctx context.Context, runner *runtimeusecase.Runner, sessions *sessionusecase.Catalog, arguments []string) error {
	selector, codexArguments, err := parseRunArguments(arguments)
	if err != nil {
		return err
	}
	if index, args := sessionArgument(codexArguments); index >= 0 {
		return runSessionArgument(ctx, sessions, selector, args, index)
	}
	if index := nativeCommandIndex(codexArguments); index >= 0 {
		if handled, err := runNativeCommand(ctx, runner, selector, codexArguments, index); handled {
			return err
		}
	}
	return runner.Run(ctx, selector, codexArguments)
}

func runSessionArgument(ctx context.Context, sessions *sessionusecase.Catalog, selector string, args []string, index int) error {
	if sessions == nil {
		return errors.New("session support is not configured")
	}
	command := nativeCommandIndex(args)
	local := command >= 0 && localSessionCommand(args[command])
	prefix, sessionSelector := splitThreadSelector(args[index])
	return sessions.ResumeArguments(ctx, sessionmodel.Launch{
		AccountSelector: selector,
		SessionSelector: sessionSelector,
		IDIndex:         index,
		IDPrefix:        prefix,
		Arguments:       args,
		Local:           local,
	})
}

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

func Doctor(ctx context.Context, doctor *runtimeusecase.Doctor, out io.Writer, arguments []string) error {
	if len(arguments) != 0 {
		return errors.New("doctor does not accept arguments")
	}
	report, err := doctor.Run(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		out,
		"Godex home: %s\nCodex: %s\nAccounts: %d (%d enabled)\n",
		report.GodexHome,
		report.CodexVersion,
		report.AccountCount,
		report.EnabledCount,
	)
	return err
}
