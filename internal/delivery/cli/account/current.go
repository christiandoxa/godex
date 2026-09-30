package account

import (
	"context"
	"errors"
	"fmt"
	"io"

	accountusecase "github.com/christiandoxa/godex/internal/usecase/account"
)

func Current(ctx context.Context, accounts Commands, out io.Writer, arguments []string) error {
	if len(arguments) != 0 {
		return errors.New("current does not accept arguments")
	}
	account, err := accountusecase.Current(ctx, accounts)
	if err != nil {
		return err
	}
	identity := account.Email
	if identity == "" {
		identity = account.ID
	}
	_, err = fmt.Fprintf(out, "%s\t%s\t%s\n", account.Name, identity, account.ID)
	return err
}
