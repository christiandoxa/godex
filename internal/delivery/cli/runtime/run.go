package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"

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
		return sessions.ResumeArguments(ctx, selector, args[index], index, args)
	}
	if len(codexArguments) > 0 {
		switch codexArguments[0] {
		case "mcp", "features", "completion", "debug", "config", "login", "logout", "--version", "version":
			return runner.RunLocal(ctx, selector, codexArguments)
		case "resume", "fork":
			return runner.RunCurrent(ctx, selector, codexArguments)
		}
	}
	return runner.Run(ctx, selector, codexArguments)
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
