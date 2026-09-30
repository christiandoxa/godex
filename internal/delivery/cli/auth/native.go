package auth

import (
	"context"
	"errors"
	"flag"
	"io"

	authmodel "github.com/christiandoxa/godex/internal/model/auth"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
)

func Native(ctx context.Context, native *authusecase.Native, logout bool, args []string) error {
	if native == nil {
		return errors.New("native authentication commands are not configured")
	}
	input := authmodel.Command{Logout: logout}
	flags := flag.NewFlagSet("auth", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&input.Selector, "account", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: godex login status/logout [--account SELECTOR]")
	}
	return native.Run(ctx, input)
}
