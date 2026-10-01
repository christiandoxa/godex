package update

import (
	"context"
	"fmt"
	"io"

	updatemodel "github.com/christiandoxa/godex/internal/model/update"
)

type statusRunner interface {
	Status(context.Context) (updatemodel.Report, error)
}

func Notice(ctx context.Context, updater statusRunner, out io.Writer) error {
	if updater == nil || out == nil {
		return nil
	}
	report, err := updater.Status(ctx)
	if err != nil || report.Status != updatemodel.UpdateAvailable {
		return err
	}
	_, err = fmt.Fprintf(
		out,
		"Update Available\nA newer godex release is available: %s -> %s\nUpdate with: godex update\n",
		report.Installed,
		report.Latest,
	)
	return err
}
