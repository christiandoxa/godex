package quota

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

type statusRunner interface {
	Run(context.Context, quotausecase.Options) ([]quotamodel.Report, error)
}

type showOptions struct {
	quotausecase.Options
	detail bool
}

func Show(ctx context.Context, status statusRunner, out io.Writer, arguments []string) error {
	options, err := parseArguments(arguments)
	if err != nil {
		return err
	}
	reports, err := status.Run(ctx, options.Options)
	if err != nil {
		return err
	}
	header := "ACCOUNT\tCURRENT\tSTATE\tPLAN\t5H\tWEEKLY"
	if options.detail {
		header += "\t5H_RESET_AT\t5H_WINDOW_SECONDS\tWEEKLY_RESET_AT\tWEEKLY_WINDOW_SECONDS"
	}
	if _, err := fmt.Fprintln(out, header); err != nil {
		return err
	}
	for _, report := range reports {
		if err := writeReport(out, report, options.detail); err != nil {
			return err
		}
	}
	return nil
}

func parseArguments(arguments []string) (showOptions, error) {
	options := showOptions{}
	for _, argument := range arguments {
		switch argument {
		case "--all":
			options.All = true
		case "--detail":
			options.detail = true
		case "--once":
			// Godex quota output is intentionally one-shot; accept Prodex's explicit spelling.
		case "--help", "-h":
			return showOptions{}, errors.New("usage: godex quota [--all] [--detail] [--once] [selector]")
		default:
			if strings.HasPrefix(argument, "-") {
				return showOptions{}, fmt.Errorf("unknown quota option %q", argument)
			}
			if options.Selector != "" {
				return showOptions{}, errors.New("quota accepts at most one account selector")
			}
			options.Selector = argument
		}
	}
	if options.All && options.Selector != "" {
		return showOptions{}, errors.New("quota selector cannot be combined with --all")
	}
	return options, nil
}

func writeReport(out io.Writer, report quotamodel.Report, detail bool) error {
	current := ""
	if report.Active {
		current = "*"
	}
	state := valueOrDash(report.State)
	usage := report.Usage
	if report.Err != nil {
		state = "error"
		usage = quotamodel.Usage{}
	}
	fields := []string{report.AccountName, current, state, valueOrDash(usage.PlanType), formatWindow(usage.Primary), formatWindow(usage.Secondary)}
	if detail {
		for _, window := range []*quotamodel.Window{usage.Primary, usage.Secondary} {
			reset, seconds := formatWindowDetails(window)
			fields = append(fields, reset, seconds)
		}
	}
	_, err := fmt.Fprintln(out, strings.Join(fields, "\t"))
	return err
}

func formatWindowDetails(window *quotamodel.Window) (reset, seconds string) {
	reset, seconds = "-", "-"
	if window == nil {
		return reset, seconds
	}
	if window.ResetAt != nil {
		reset = time.Unix(*window.ResetAt, 0).UTC().Format(time.RFC3339)
	}
	if window.LimitWindowSeconds != nil {
		seconds = strconv.FormatInt(*window.LimitWindowSeconds, 10)
	}
	return reset, seconds
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
