package account

import (
	"context"
	"errors"
	"fmt"
	"io"

	accountusecase "github.com/christiandoxa/godex/internal/usecase/account"
)

func Use(ctx context.Context, accounts Commands, out io.Writer, arguments []string) error {
	if len(arguments) != 1 {
		return errors.New("usage: godex account use <id|name|email>")
	}
	account, err := accountusecase.Use(ctx, accounts, arguments[0])
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Next launch will start with %s.\n", account.Name)
	return err
}
