package quota

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

var quotaWatchInterval = 5 * time.Second

func watchQuota(ctx context.Context, status statusRunner, out io.Writer, options showOptions) error {
	if terminalFile(os.Stdin) && quotaTerminalWriter(out) {
		return runQuotaTUI(ctx, status, out, options)
	}
	var previous []quotamodel.Report
	for {
		var err error
		previous, err = writeQuotaWatchIteration(ctx, status, out, options, previous)
		if err != nil {
			return err
		}
		if err := waitQuotaRefresh(ctx); err != nil {
			return err
		}
	}
}

func writeQuotaWatchIteration(
	ctx context.Context,
	status statusRunner,
	out io.Writer,
	options showOptions,
	previous []quotamodel.Report,
) ([]quotamodel.Report, error) {
	reports, err := status.Run(ctx, options.Options)
	if err == nil {
		previous = reports
	}
	if err := writeQuotaWatchSnapshot(out, previous, options.detail); err != nil {
		return previous, err
	}
	return previous, nil
}

func writeQuotaWatchSnapshot(out io.Writer, reports []quotamodel.Report, detail bool) error {
	if len(reports) > 0 {
		if err := writeQuotaReports(out, reports, detail); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintln(out, "Quota unavailable"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if flusher, ok := out.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

func waitQuotaRefresh(ctx context.Context) error {
	timer := time.NewTimer(quotaWatchInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
