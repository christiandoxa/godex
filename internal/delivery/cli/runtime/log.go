package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type logOptions struct {
	mode string
	json bool
}

func Log(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer, arguments []string) error {
	options, err := parseLogArguments(arguments)
	if err != nil {
		return err
	}
	if activity == nil {
		return errors.New("runtime activity is not configured")
	}
	events, err := initialLogEvents(ctx, activity, options)
	if err != nil {
		return err
	}
	if options.mode == "last" {
		return writeLastLogEvent(out, events, options.json)
	}
	if logTUIEnabled(options, out) {
		return runLogTUI(ctx, activity, out, options)
	}
	return followLogPlain(ctx, activity, out, options, events)
}

func initialLogEvents(ctx context.Context, activity *runtimeusecase.Activity, options logOptions) ([]runtimemodel.Event, error) {
	events, err := activity.Events(ctx, 256)
	if err != nil {
		return nil, err
	}
	return filterLogEvents(events, options.mode), nil
}

func writeLastLogEvent(out io.Writer, events []runtimemodel.Event, jsonOutput bool) error {
	if len(events) == 0 {
		return nil
	}
	return writeLogEvent(out, events[len(events)-1], jsonOutput)
}

func logTUIEnabled(options logOptions, out io.Writer) bool {
	return !options.json && writerIsTerminal(os.Stdin) && writerIsTerminal(out)
}

func followLogPlain(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer, options logOptions, events []runtimemodel.Event) error {
	seen, err := writeInitialLogEvents(out, events, options.json)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := writeNewLogEvents(ctx, activity, out, options, seen); err != nil {
				return err
			}
		}
	}
}

func writeInitialLogEvents(out io.Writer, events []runtimemodel.Event, jsonOutput bool) (map[string]bool, error) {
	seen := make(map[string]bool, len(events))
	for _, event := range events {
		if err := writeLogEvent(out, event, jsonOutput); err != nil {
			return nil, err
		}
		seen[eventFingerprint(event)] = true
	}
	return seen, nil
}

func parseLogArguments(arguments []string) (logOptions, error) {
	options := logOptions{mode: "stream"}
	for _, argument := range arguments {
		switch argument {
		case "--json":
			options.json = true
		case "stream", "last", "upstream":
			if options.mode != "stream" || argument == "stream" && hasExplicitLogMode(arguments) {
				if options.mode != argument && options.mode != "stream" {
					return logOptions{}, errors.New("log accepts at most one mode")
				}
			}
			options.mode = argument
		case "--help", "-h":
			return logOptions{}, errors.New("usage: godex log [stream|last|upstream] [--json]")
		default:
			return logOptions{}, fmt.Errorf("unknown log option %q", argument)
		}
	}
	return options, nil
}

func hasExplicitLogMode(arguments []string) bool {
	count := 0
	for _, argument := range arguments {
		if argument == "stream" || argument == "last" || argument == "upstream" {
			count++
		}
	}
	return count > 1
}

func filterLogEvents(events []runtimemodel.Event, mode string) []runtimemodel.Event {
	if mode != "upstream" {
		return events
	}
	filtered := make([]runtimemodel.Event, 0, len(events))
	for _, event := range events {
		if strings.HasPrefix(event.Kind, "request_") {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func writeNewLogEvents(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer, options logOptions, seen map[string]bool) error {
	events, err := activity.Events(ctx, 256)
	if err != nil {
		return err
	}
	for _, event := range filterLogEvents(events, options.mode) {
		key := eventFingerprint(event)
		if seen[key] {
			continue
		}
		if err := writeLogEvent(out, event, options.json); err != nil {
			return err
		}
		seen[key] = true
	}
	return nil
}

func eventFingerprint(event runtimemodel.Event) string {
	return fmt.Sprintf("%d/%s/%s", event.TimestampUnixMilli, event.RequestID, event.Kind)
}

func writeLogEvent(out io.Writer, event runtimemodel.Event, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(out).Encode(event)
	}
	_, err := fmt.Fprintln(out, formatLogEvent(event))
	return err
}

func formatLogEvent(event runtimemodel.Event) string {
	when := time.UnixMilli(event.TimestampUnixMilli).Format(time.RFC3339Nano)
	return fmt.Sprintf("%s\t%s\tstatus=%d\taccount=%s\t%s %s\tduration_ms=%d\t%s",
		when, event.Kind, event.StatusCode, valueOrDash(event.AccountID),
		event.Method, event.Path, event.DurationMillis, event.Message)
}
