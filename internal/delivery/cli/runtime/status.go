package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type statusOptions struct {
	once     bool
	interval time.Duration
}

func Status(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer, arguments []string) error {
	options, err := parseStatusArguments(arguments)
	if err != nil {
		return err
	}
	if activity == nil {
		return errors.New("runtime activity is not configured")
	}
	if options.once || !writerIsTerminal(out) {
		return writeStatusSnapshot(ctx, activity, out)
	}
	if err := writeLiveStatusSnapshot(ctx, activity, out); err != nil {
		return err
	}
	ticker := time.NewTicker(options.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := writeLiveStatusSnapshot(ctx, activity, out); err != nil {
				return err
			}
		}
	}
}

func parseStatusArguments(arguments []string) (statusOptions, error) {
	options := statusOptions{interval: time.Second}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--once":
			options.once = true
		case argument == "--help" || argument == "-h":
			return statusOptions{}, errors.New("usage: godex status [--once] [--interval SECONDS]")
		case argument == "--interval":
			if index+1 >= len(arguments) {
				return statusOptions{}, errors.New("--interval requires seconds")
			}
			index++
			seconds, err := parseStatusInterval(arguments[index])
			if err != nil {
				return statusOptions{}, err
			}
			options.interval = time.Duration(seconds) * time.Second
		case strings.HasPrefix(argument, "--interval="):
			seconds, err := parseStatusInterval(strings.TrimPrefix(argument, "--interval="))
			if err != nil {
				return statusOptions{}, err
			}
			options.interval = time.Duration(seconds) * time.Second
		default:
			return statusOptions{}, fmt.Errorf("unknown status option %q", argument)
		}
	}
	return options, nil
}

func parseStatusInterval(value string) (int64, error) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 1 || seconds > 60 {
		return 0, fmt.Errorf("invalid --interval value %q", value)
	}
	return seconds, nil
}

func writeLiveStatusSnapshot(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer) error {
	if _, err := io.WriteString(out, "\x1b[H\x1b[2J"); err != nil {
		return err
	}
	return writeStatusSnapshot(ctx, activity, out)
}

func writeStatusSnapshot(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer) error {
	overview, err := activity.Overview(ctx)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Updated: %s\n", time.Now().Format("2006-01-02 15:04:05")); err != nil {
		return err
	}
	for _, field := range statusFields(overview) {
		if _, err := fmt.Fprintf(out, "%s: %s\n", field[0], field[1]); err != nil {
			return err
		}
	}
	return nil
}

func statusFields(overview runtimemodel.Overview) [][2]string {
	fields := [][2]string{
		{"Active profile", valueOrDash(overview.ActiveAccount)},
		{"Profiles", fmt.Sprint(overview.AccountCount)},
		{"Enabled", fmt.Sprint(overview.EnabledCount)},
		{"Inflight", fmt.Sprint(overview.Inflight)},
		{"Recent events", fmt.Sprint(overview.RecentEvents)},
	}
	if overview.LastEvent != nil {
		last := overview.LastEvent
		fields = append(fields,
			[2]string{"Last event", last.Kind},
			[2]string{"Last status", fmt.Sprint(last.StatusCode)},
			[2]string{"Last event at", time.UnixMilli(last.TimestampUnixMilli).Format(time.RFC3339)},
		)
	}
	return fields
}

func writerIsTerminal(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
