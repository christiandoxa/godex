package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	events, err := activity.Events(ctx, 256)
	if err != nil {
		return err
	}
	events = filterLogEvents(events, options.mode)
	if options.mode == "last" {
		if len(events) == 0 {
			return nil
		}
		return writeLogEvent(out, events[len(events)-1], options.json)
	}
	seen := make(map[string]bool, len(events))
	for _, event := range events {
		if err := writeLogEvent(out, event, options.json); err != nil {
			return err
		}
		seen[eventFingerprint(event)] = true
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
	when := time.UnixMilli(event.TimestampUnixMilli).Format(time.RFC3339Nano)
	_, err := fmt.Fprintf(out, "%s\t%s\tstatus=%d\taccount=%s\t%s %s\tduration_ms=%d\t%s\n",
		when, event.Kind, event.StatusCode, valueOrDash(event.AccountID),
		event.Method, event.Path, event.DurationMillis, event.Message)
	return err
}
