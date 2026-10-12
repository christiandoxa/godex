package account

import (
	"context"
	"errors"
	"fmt"
	"io"

	usagecli "github.com/christiandoxa/godex/internal/delivery/cli/usage"
	accountusecase "github.com/christiandoxa/godex/internal/usecase/account"
)

type Commands = accountusecase.AccountStore

func Run(ctx context.Context, accounts Commands, out io.Writer, arguments []string) error {
	if len(arguments) == 0 {
		return usagecli.Wrap(errors.New("account requires list, current, use, enable, disable, remove, or import-current"))
	}
	switch arguments[0] {
	case "list":
		return List(ctx, accounts, out, arguments[1:])
	case "current":
		return Current(ctx, accounts, out, arguments[1:])
	case "use":
		return Use(ctx, accounts, out, arguments[1:])
	case "enable", "disable":
		return Enable(ctx, accounts, out, arguments[1:], arguments[0] == "enable")
	case "remove":
		return Remove(ctx, accounts, out, arguments[1:])
	default:
		return usagecli.Wrap(fmt.Errorf("unknown account command %q", arguments[0]))
	}
}
