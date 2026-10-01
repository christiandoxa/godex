package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	updatemodel "github.com/christiandoxa/godex/internal/model/update"
)

type runner interface {
	Run(context.Context) (updatemodel.Report, error)
}

func Run(ctx context.Context, updater runner, out, errOut io.Writer, arguments []string) error {
	if len(arguments) != 0 {
		return errors.New("update does not accept arguments")
	}
	if updater == nil {
		return errors.New("Godex update support is not configured")
	}
	report, runErr := updater.Run(ctx)
	if report.Latest != "" {
		if err := writeReport(out, report); err != nil {
			return err
		}
	}
	if strings.TrimSpace(report.Stdout) != "" {
		if _, err := fmt.Fprintln(out, report.Stdout); err != nil {
			return err
		}
	}
	if strings.TrimSpace(report.Stderr) != "" {
		if _, err := fmt.Fprintln(errOut, report.Stderr); err != nil {
			return err
		}
	}
	return runErr
}

func writeReport(out io.Writer, report updatemodel.Report) error {
	status := string(report.Status)
	message := ""
	switch report.Status {
	case updatemodel.UpToDate:
		status = "up to date"
		message = fmt.Sprintf("Godex %s is already up to date.", report.Installed)
	case updatemodel.LocalNewer:
		status = "local version is newer"
		message = fmt.Sprintf("Installed Godex %s is newer than latest stable %s. No changes made.", report.Installed, report.Latest)
	case updatemodel.UpdateAvailable:
		status = "updating"
		message = fmt.Sprintf("Updating Godex %s → %s...", report.Installed, report.Latest)
	case updatemodel.Updated:
		status = "updated"
		message = fmt.Sprintf("Godex updated to %s.", report.Latest)
	}
	_, err := fmt.Fprintf(out, "Godex Update\nInstalled: %s\nLatest: %s\nStatus: %s\n%s\n", report.Installed, report.Latest, status, message)
	return err
}
