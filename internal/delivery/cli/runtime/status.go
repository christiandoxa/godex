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
	if options.once || !writerIsTerminal(os.Stdin) || !writerIsTerminal(out) {
		return writeStatusSnapshot(ctx, activity, out)
	}
	return runStatusTUI(ctx, activity, out, options.interval)
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

func writeStatusSnapshot(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer) error {
	overview, err := activity.Overview(ctx)
	if err != nil {
		return err
	}
	resources := newStatusResourceTracker()
	_ = resources.sample()
	timer := time.NewTimer(200 * time.Millisecond)
	select {
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	}
	resourceSnapshot := resources.sample()
	for _, field := range statusFields(overview, resourceSnapshot) {
		if _, err := fmt.Fprintf(out, "%s: %s\n", field[0], field[1]); err != nil {
			return err
		}
	}
	return nil
}

func statusFields(overview runtimemodel.Overview, resources statusResourceSnapshot) [][2]string {
	updated := overview.UpdatedAt
	if strings.TrimSpace(updated) == "" {
		updated = time.Now().In(time.Local).Format("2006-01-02 15:04:05")
	}
	return [][2]string{
		{"Profile", statusProfileField(overview)},
		{"5h quota", statusPoolRemaining(overview.Quota.FiveHour)},
		{"5h runway", statusRunway(overview.Quota.FiveHour, overview.FiveHourRunway, overview.UpdatedUnix)},
		{"Weekly quota", statusPoolRemaining(overview.Quota.Weekly)},
		{"Weekly runway", statusRunway(overview.Quota.Weekly, overview.WeeklyRunway, overview.UpdatedUnix)},
		{"Token usage", statusTokenUsage(overview.TokenSummary)},
		{"Token efficiency", statusTokenEfficiency(overview.TokenSummary.Total)},
		{"Token history", statusTokenHistory(overview)},
		{"Processes", statusProcessField(resources)},
		{"Memory", statusMemoryField(resources)},
		{"Network", statusNetworkField(resources)},
		{"Disk I/O", statusDiskField(resources)},
		{"Recent load", statusLoadSummary(overview.RuntimeLoad, statusRuntimeProcessCount(resources))},
		{"Updated", updated},
	}
}

func writerIsTerminal(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func statusProcessField(resources statusResourceSnapshot) string {
	if !resources.available {
		return "unavailable"
	}
	cpu := "warming up"
	if resources.cpuPercent != nil {
		cpu = fmt.Sprintf("%.1f%%", *resources.cpuPercent)
	}
	return fmt.Sprintf("%d total, %d runtime; CPU %s",
		resources.processCount, resources.runtimeProcessCount, cpu)
}

func statusMemoryField(resources statusResourceSnapshot) string {
	if !resources.available {
		return "unavailable"
	}
	return fmt.Sprintf("%s (%.1f%% host)",
		statusHumanBytes(resources.residentBytes), statusMemoryPercent(resources))
}

func statusNetworkField(resources statusResourceSnapshot) string {
	if !resources.available {
		return "unavailable"
	}
	return fmt.Sprintf("%d sockets; RX queue %s, TX queue %s",
		resources.socketCount,
		statusHumanBytes(resources.networkRXQueueBytes),
		statusHumanBytes(resources.networkTXQueueBytes))
}

func statusDiskField(resources statusResourceSnapshot) string {
	if !resources.available {
		return "unavailable"
	}
	return fmt.Sprintf("read %s total (%s/s), write %s total (%s/s)",
		statusHumanBytes(resources.diskReadBytes),
		statusHumanBytes(resources.diskReadBytesPerSecond),
		statusHumanBytes(resources.diskWriteBytes),
		statusHumanBytes(resources.diskWriteBytesPerSecond))
}
