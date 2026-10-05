package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	authmodel "github.com/christiandoxa/godex/internal/model/auth"
)

type currentImporter interface {
	Run(context.Context, authmodel.ImportCurrentRequest) (authmodel.ImportCurrentResponse, error)
}

func ImportCurrent(
	ctx context.Context,
	importer currentImporter,
	out io.Writer,
	arguments []string,
) error {
	if importer == nil {
		return errors.New("current Codex profile importer is not configured")
	}
	request := authmodel.ImportCurrentRequest{Name: "default"}
	nameSet := false
	for _, argument := range arguments {
		switch {
		case argument == "--insecure":
			request.Insecure = true
		case strings.HasPrefix(argument, "-"):
			return fmt.Errorf("unknown import-current option %q", argument)
		case !nameSet:
			request.Name = argument
			nameSet = true
		default:
			return errors.New("profile import-current accepts at most one name")
		}
	}
	account, err := importer.Run(ctx, request)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		out, "Imported current Codex login as %s (%s).\n",
		account.Name, displayIdentity(account.Email, account.ID),
	)
	return err
}
