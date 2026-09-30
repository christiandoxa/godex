package account

import (
	"context"
	"errors"
	"fmt"
	"io"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	accountusecase "github.com/christiandoxa/godex/internal/usecase/account"
)

func Enable(ctx context.Context, accounts Commands, out io.Writer, args []string, enabled bool) error {
	if len(args) != 1 {
		return errors.New("usage: godex account enable/disable SELECTOR")
	}
	store, ok := accounts.(interface {
		SetEnabled(context.Context, string, bool) (accountentity.Account, error)
	})
	if !ok {
		return errors.New("account enablement is not configured")
	}
	account, err := accountusecase.Enable(ctx, store, args[0], enabled)
	if err != nil {
		return err
	}
	status := "disabled"
	if enabled {
		status = "enabled"
	}
	_, err = fmt.Fprintf(out, "Account %s is %s.\n", account.Name, status)
	return err
}
