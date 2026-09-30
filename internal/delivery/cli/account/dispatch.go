package account

import (
	"context"
	"errors"
	"fmt"
	"io"

	accountusecase "github.com/christiandoxa/godex/internal/usecase/account"
)

type Commands = accountusecase.AccountStore

func Run(ctx context.Context, accounts Commands, out io.Writer, arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("account requires list, current, use, remove, or import-current")
	}
	switch arguments[0] {
	case "list":
		return List(ctx, accounts, out, arguments[1:])
	case "current":
		return Current(ctx, accounts, out, arguments[1:])
	case "use":
		return Use(ctx, accounts, out, arguments[1:])
	case "remove":
		return Remove(ctx, accounts, out, arguments[1:])
	default:
		return fmt.Errorf("unknown account command %q", arguments[0])
	}
}
