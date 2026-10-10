package quota

import (
	"bytes"
	"context"
	"encoding/json"
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
	Raw(context.Context, string, string) ([]byte, error)
	HasProfiles(context.Context) (bool, error)
}

func Show(ctx context.Context, status statusRunner, out io.Writer, arguments []string) error {
	options, err := parseArguments(arguments)
	if err != nil {
		return err
	}
	if options.raw {
		return showRaw(ctx, status, out, options)
	}
	if options.watchEnabled() {
		return watchQuota(ctx, status, out, options)
	}
	reports, err := status.Run(ctx, options.Options)
	if err != nil {
		return err
	}
	if options.once && len(reports) == 0 {
		hasProfiles, err := status.HasProfiles(ctx)
		if err != nil {
			return err
		}
		if !hasProfiles {
			return errors.New("no profiles configured")
		}
	}
	return writeQuotaReports(out, reports, options.detail)
}

func showRaw(ctx context.Context, status statusRunner, out io.Writer, options showOptions) error {
	body, err := status.Raw(ctx, options.Selector, options.BaseURL)
	if err != nil {
		return err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		return errors.New("decode quota response")
	}
	pretty.WriteByte('\n')
	_, err = out.Write(pretty.Bytes())
	return err
}

func writeQuotaReports(out io.Writer, reports []quotamodel.Report, detail bool) error {
	header := "PROFILE\tCURRENT\tPROVIDER\tAUTH\tSTATE\tPLAN\t5H\tWEEKLY"
	if detail {
		header += "\t5H_RESET_AT\t5H_WINDOW_SECONDS\tWEEKLY_RESET_AT\tWEEKLY_WINDOW_SECONDS"
	}
	if _, err := fmt.Fprintln(out, header); err != nil {
		return err
	}
	for _, report := range reports {
		if err := writeReport(out, report, detail); err != nil {
			return err
		}
	}
	return nil
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
	name := report.ProfileName
	if name == "" {
		name = report.AccountName
	}
	plan, primary, secondary := quotaReportRowValues(report, usage)
	fields := []string{name, current, valueOrDash(report.Provider), valueOrDash(report.Auth), state, valueOrDash(plan), primary, secondary}
	if detail {
		for _, window := range []*quotamodel.Window{usage.Primary, usage.Secondary} {
			reset, seconds := formatWindowDetails(window)
			fields = append(fields, reset, seconds)
		}
	}
	_, err := fmt.Fprintln(out, strings.Join(fields, "\t"))
	return err
}

func quotaReportRowValues(report quotamodel.Report, usage quotamodel.Usage) (plan, primary, secondary string) {
	if report.External != nil && report.Err == nil {
		return report.External.Plan, valueOrDash(report.External.Main), valueOrDash(report.External.Reset)
	}
	return usage.PlanType, formatWindow(usage.Primary), formatWindow(usage.Secondary)
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
