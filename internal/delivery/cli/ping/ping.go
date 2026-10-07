package ping

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
	pingusecase "github.com/christiandoxa/godex/internal/usecase/ping"
)

type runner interface {
	Run(context.Context, pingmodel.Options) (pingmodel.Report, error)
}

type observedRunner interface {
	RunObserved(context.Context, pingmodel.Options, func(pingmodel.Result) error) (pingmodel.Report, error)
}

func Run(ctx context.Context, probe runner, out io.Writer, arguments []string) error {
	if probe == nil {
		return errors.New("ping support is not configured")
	}
	options, err := parseArguments(arguments)
	if err != nil {
		return err
	}
	if err := pingusecase.ValidateOptions(options); err != nil {
		return err
	}
	if pingShouldPromptSelection(options.JSON, pingInteractiveTerminal()) {
		options, err = promptPingOptions(ctx, options)
		if err != nil {
			return err
		}
	}
	report, err := runProbe(ctx, probe, out, options)
	if err != nil {
		return err
	}
	if options.JSON {
		if err := writeJSON(out, report); err != nil {
			return err
		}
	} else if err := writeSummary(out, report.Summary); err != nil {
		return err
	}
	if pingusecase.Failed(report) {
		return errors.New("OpenAI application ping failed: one or more profiles are not healthy")
	}
	return nil
}

func parseArguments(arguments []string) (pingmodel.Options, error) {
	if len(arguments) == 0 || arguments[0] != "openai" {
		return pingmodel.Options{}, errors.New("usage: godex ping openai [-p|--profile NAME] [--model MODEL] [--effort LEVEL] [--base-url URL] [--no-proxy] [--json]")
	}
	options := pingmodel.Options{}
	for index := 1; index < len(arguments); index++ {
		next, err := consumeArgument(arguments, index, &options)
		if err != nil {
			return pingmodel.Options{}, err
		}
		index = next
	}
	return options, nil
}

func consumeArgument(arguments []string, index int, options *pingmodel.Options) (int, error) {
	argument := arguments[index]
	switch argument {
	case "--no-proxy":
		options.NoProxy = true
		return index, nil
	case "--json":
		options.JSON = true
		return index, nil
	case "--help", "-h":
		return index, errors.New("usage: godex ping openai [-p|--profile NAME] [--model MODEL] [--effort LEVEL] [--base-url URL] [--no-proxy] [--json]")
	}
	for _, option := range []struct {
		name string
		set  func(string)
	}{{"--profile", func(value string) { options.Profile = value }}, {"-p", func(value string) { options.Profile = value }}, {"--model", func(value string) { options.Model = value }}, {"--effort", func(value string) { options.Effort = value }}, {"--base-url", func(value string) { options.BaseURL = value }}} {
		value, next, handled, err := optionValue(arguments, index, option.name)
		if !handled {
			continue
		}
		if err != nil {
			return index, err
		}
		option.set(value)
		return next, nil
	}
	return index, fmt.Errorf("unknown ping option %q", argument)
}

func optionValue(arguments []string, index int, name string) (string, int, bool, error) {
	argument := arguments[index]
	if argument == name {
		if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
			return "", index, true, fmt.Errorf("%s requires a value", name)
		}
		return arguments[index+1], index + 1, true, nil
	}
	prefix := name + "="
	if strings.HasPrefix(argument, prefix) {
		value := strings.TrimSpace(strings.TrimPrefix(argument, prefix))
		if value == "" {
			return "", index, true, fmt.Errorf("%s requires a value", name)
		}
		return value, index, true, nil
	}
	return "", index, false, nil
}

func runProbe(ctx context.Context, probe runner, out io.Writer, options pingmodel.Options) (pingmodel.Report, error) {
	if options.JSON {
		return probe.Run(ctx, options)
	}
	if _, err := fmt.Fprintln(out, "OpenAI application ping"); err != nil {
		return pingmodel.Report{}, err
	}
	if observed, ok := probe.(observedRunner); ok {
		return observed.RunObserved(ctx, options, func(result pingmodel.Result) error {
			return writeProfileResult(out, result)
		})
	}
	report, err := probe.Run(ctx, options)
	if err != nil {
		return pingmodel.Report{}, err
	}
	for _, result := range report.Profiles {
		if err := writeProfileResult(out, result); err != nil {
			return pingmodel.Report{}, err
		}
	}
	return report, nil
}

func writeProfileResult(out io.Writer, result pingmodel.Result) error {
	first := latencyLabel(result.FirstResponseLatencyMS, "unavailable")
	completion := latencyValue(result.CompletionLatencyMS)
	requested := valueOrDefault(result.RequestedModel, "configured/default")
	effort := valueOrDefault(result.RequestedEffort, "configured/default")
	effective := valueOrDefault(result.EffectiveModel, "unavailable")
	if _, err := fmt.Fprintf(
		out, "%s  %-20s first=%s completion=%dms  requested=%s effort=%s effective=%s\n",
		result.Profile, pingusecase.HumanStatus(result.Status), first, completion, requested, effort, effective,
	); err != nil {
		return err
	}
	if result.Status == pingmodel.Pass {
		return nil
	}
	_, err := fmt.Fprintf(out, "  reason: %s\n", result.Detail)
	return err
}

func latencyLabel(value *int64, fallback string) string {
	if value == nil {
		return fallback
	}
	return fmt.Sprintf("%dms", *value)
}

func latencyValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func writeSummary(out io.Writer, summary pingmodel.Summary) error {
	fields := [][2]string{
		{"Profiles discovered", fmt.Sprint(summary.ProfilesDiscovered)},
		{"Profiles tested", fmt.Sprint(summary.ProfilesTested)},
		{"Healthy", fmt.Sprint(summary.Healthy)},
		{"Exhausted", fmt.Sprint(summary.Exhausted)},
		{"Auth failures", fmt.Sprint(summary.AuthFailures)},
		{"Temporary failures", fmt.Sprint(summary.TemporaryFailures)},
		{"Other failures", fmt.Sprint(summary.OtherFailures)},
		{"Total duration", fmt.Sprintf("%dms", summary.DurationMS)},
		{"Pool usable", map[bool]string{true: "yes", false: "no"}[summary.PoolUsable]},
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(out, "%s: %s\n", field[0], field[1]); err != nil {
			return err
		}
	}
	return nil
}
