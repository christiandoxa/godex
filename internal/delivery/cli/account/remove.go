package account

import (
	"context"
	"errors"
	"fmt"
	"io"

	accountusecase "github.com/christiandoxa/godex/internal/usecase/account"
)

func Remove(ctx context.Context, accounts Commands, out io.Writer, arguments []string) error {
	if len(arguments) != 1 {
		return errors.New("usage: godex account remove <id|name|email>")
	}
	account, err := accountusecase.Remove(ctx, accounts, arguments[0])
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Removed %s.\n", account.Name)
	return err
}
