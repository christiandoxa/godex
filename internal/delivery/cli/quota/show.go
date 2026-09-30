package quota

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

type statusRunner interface {
	Run(context.Context, quotausecase.Options) ([]quotamodel.Report, error)
}

func Show(ctx context.Context, status statusRunner, out io.Writer, arguments []string) error {
	options, err := parseArguments(arguments)
	if err != nil {
		return err
	}
	reports, err := status.Run(ctx, options)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "ACCOUNT\tCURRENT\tSTATE\tPLAN\t5H\tWEEKLY"); err != nil {
		return err
	}
	for _, report := range reports {
		if err := writeReport(out, report); err != nil {
			return err
		}
	}
	return nil
}

func parseArguments(arguments []string) (quotausecase.Options, error) {
	options := quotausecase.Options{}
	for _, argument := range arguments {
		switch argument {
		case "--all":
			options.All = true
		case "--once":
			// Godex quota output is intentionally one-shot; accept Prodex's explicit spelling.
		case "--help", "-h":
			return quotausecase.Options{}, errors.New("usage: godex quota [--all] [--once] [selector]")
		default:
			if strings.HasPrefix(argument, "-") {
				return quotausecase.Options{}, fmt.Errorf("unknown quota option %q", argument)
			}
			if options.Selector != "" {
				return quotausecase.Options{}, errors.New("quota accepts at most one account selector")
			}
			options.Selector = argument
		}
	}
	if options.All && options.Selector != "" {
		return quotausecase.Options{}, errors.New("quota selector cannot be combined with --all")
	}
	return options, nil
}

func writeReport(out io.Writer, report quotamodel.Report) error {
	current := ""
	if report.Active {
		current = "*"
	}
	state := valueOrDash(report.State)
	plan := valueOrDash(report.Usage.PlanType)
	fiveHour := formatWindow(report.Usage.Primary)
	weekly := formatWindow(report.Usage.Secondary)
	if report.Err != nil {
		state = "error"
		plan = "-"
		fiveHour = "-"
		weekly = "-"
	}
	_, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\n", report.AccountName, current, state, plan, fiveHour, weekly)
	return err
}

func formatWindow(window *quotamodel.Window) string {
	if window == nil || window.UsedPercent == nil {
		return "-"
	}
	remaining := 100 - *window.UsedPercent
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	return fmt.Sprintf("%d%%", remaining)
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
