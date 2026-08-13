package account

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	accountusecase "github.com/christiandoxa/godex/internal/usecase/account"
)

func List(ctx context.Context, accounts Commands, out io.Writer, arguments []string) error {
	if len(arguments) != 0 {
		return errors.New("accounts does not accept arguments")
	}
	listed, err := accountusecase.List(ctx, accounts)
	if err != nil {
		return err
	}
	if len(listed) == 0 {
		_, err = fmt.Fprintln(out, "No accounts. Run `godex login`.")
		return err
	}

	writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tEMAIL\tID\tSTATUS"); err != nil {
		return err
	}
	for _, account := range listed {
		status := "enabled"
		if !account.Enabled {
			status = "disabled"
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", account.Name, account.Email, account.ID, status); err != nil {
			return err
		}
	}
	return writer.Flush()
}
