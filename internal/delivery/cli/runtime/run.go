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
		if sessions == nil {
			return errors.New("session support is not configured")
		}
		command := nativeCommandIndex(args)
		local := command >= 0 && (args[command] == "delete" || args[command] == "archive" || args[command] == "unarchive")
		prefix, sessionSelector := "", args[index]
		if value, ok := strings.CutPrefix(sessionSelector, "--thread="); ok {
			prefix, sessionSelector = "--thread=", value
		}
		return sessions.ResumeArguments(ctx, sessionmodel.Launch{AccountSelector: selector, SessionSelector: sessionSelector, IDIndex: index, IDPrefix: prefix, Arguments: args, Local: local})
	}
	if index := nativeCommandIndex(codexArguments); index >= 0 {
		if unsafeNativeCommand(codexArguments, index) {
			return errors.New("native command bypasses Godex routing; use godex exec or godex app-server without daemon/proxy")
		}
		switch codexArguments[index] {
		case "logout":
			return errors.New("use godex logout to safely mutate managed credentials")
		case "login":
			if status := nextCommandWord(codexArguments, index+1); status < 0 || codexArguments[status] != "status" {
				return errors.New("use godex login to register and safely update managed credentials")
			}
			return runner.RunLocal(ctx, selector, codexArguments)
		case "mcp", "features", "completion", "debug", "config", "delete", "archive", "unarchive", "version", "--version":
			return runner.RunLocal(ctx, selector, codexArguments)
		case "resume", "fork", "queue":
			return runner.RunCurrent(ctx, selector, codexArguments)
		}
	}
	return runner.Run(ctx, selector, codexArguments)
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
