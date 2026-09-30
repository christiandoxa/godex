package auth

import (
	"context"
	"errors"
	"fmt"
	"io"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type currentImporter interface {
	Run(context.Context, string) (accountentity.Account, error)
}

func ImportCurrent(ctx context.Context, importer currentImporter, out io.Writer, arguments []string) error {
	if importer == nil {
		return errors.New("current Codex profile importer is not configured")
	}
	if len(arguments) > 1 {
		return errors.New("usage: godex profile import-current [name]")
	}
	name := ""
	if len(arguments) == 1 {
		name = arguments[0]
	}
	account, err := importer.Run(ctx, name)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Imported current Codex login as %s (%s).\n", account.Name, displayIdentity(account.Email, account.ID))
	return err
}
