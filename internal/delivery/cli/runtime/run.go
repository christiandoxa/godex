package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"

	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func Launch(ctx context.Context, runner *runtimeusecase.Runner) error {
	return runner.Run(ctx, "", nil)
}

func Run(ctx context.Context, runner *runtimeusecase.Runner, arguments []string) error {
	selector, codexArguments, err := parseRunArguments(arguments)
	if err != nil {
		return err
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
