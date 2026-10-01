package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	"github.com/christiandoxa/godex/internal/version"
)

func Info(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer, arguments []string) error {
	jsonOutput, tokens, err := parseInfoArguments(arguments)
	if err != nil {
		return err
	}
	if activity == nil {
		return errors.New("runtime activity is not configured")
	}
	overview, err := activity.Overview(ctx)
	if err != nil {
		return err
	}
	if jsonOutput {
		value := map[string]any{
			"version":               version.String(),
			"active_profile":        overview.ActiveProfile,
			"profile_count":         overview.ProfileCount,
			"provider":              "openai",
			"runtime_process_count": boolToInt(overview.Inflight > 0),
			"runtime_load": map[string]any{
				"active_inflight_units":   overview.Inflight,
				"recent_selection_events": overview.RecentEvents,
			},
			"overview": overview,
		}
		if tokens {
			value["token_usage"] = "unavailable"
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	fields := [][2]string{
		{"Profiles", fmt.Sprint(overview.ProfileCount)},
		{"Active profile", valueOrDash(overview.ActiveProfile)},
		{"Provider", "openai"},
		{"Enabled profiles", fmt.Sprint(overview.EnabledCount)},
		{"Runtime inflight", fmt.Sprint(overview.Inflight)},
		{"Recent runtime events", fmt.Sprint(overview.RecentEvents)},
		{"Godex version", version.String()},
		{"Codex version", overview.CodexVersion},
		{"Godex home", overview.GodexHome},
	}
	if tokens {
		fields = append(fields, [2]string{"Token usage", "unavailable"})
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(out, "%s: %s\n", field[0], field[1]); err != nil {
			return err
		}
	}
	return nil
}

func parseInfoArguments(arguments []string) (jsonOutput, tokens bool, err error) {
	for _, argument := range arguments {
		switch argument {
		case "--json":
			jsonOutput = true
		case "--tokens":
			tokens = true
		case "--help", "-h":
			return false, false, errors.New("usage: godex info [--json] [--tokens]")
		default:
			return false, false, fmt.Errorf("unknown info option %q", argument)
		}
	}
	return jsonOutput, tokens, nil
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
