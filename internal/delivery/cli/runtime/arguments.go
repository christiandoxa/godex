package runtime

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const (
	providerModelOptionName   = "model"
	providerContextOptionName = "context-window"
	providerCompactOptionName = "auto-compact-token-limit"
)

const optionRequiresValueFormat = "%s requires a value"

func parseRunArguments(arguments []string) (runtimemodel.Selection, []string, error) {
	arguments = NormalizeNativeAntigravityArguments(arguments)
	if UsesNativeAntigravity(arguments) {
		return parseNativeAntigravityArguments(arguments)
	}
	selection := runtimemodel.Selection{}
	features := runtimeFeatures{}
	for index := 0; index < len(arguments); {
		if arguments[index] == "--" {
			return finishRunArguments(selection, features, arguments[index+1:])
		}
		next, handled, err := consumeWrapperArgument(arguments, index, &selection, &features)
		if err != nil {
			return runtimemodel.Selection{}, nil, err
		}
		if !handled {
			return finishRunArguments(selection, features, arguments[index:])
		}
		index = next
	}
	return finishRunArguments(selection, features, nil)
}

func consumeWrapperArgument(
	arguments []string,
	index int,
	selection *runtimemodel.Selection,
	features *runtimeFeatures,
) (int, bool, error) {
	if arguments[index] == "--dry-run" {
		selection.DryRun = true
		return index + 1, true, nil
	}
	if arguments[index] == "--auto-redeem" {
		selection.AutoRedeem = true
		return index + 1, true, nil
	}
	if next, handled, err := consumeSelectorArgument(arguments, index, selection); handled {
		return next, true, err
	}
	if next, handled, err := consumeProviderArgument(arguments, index, selection); handled {
		return next, true, err
	}
	return features.consume(arguments, index)
}

func consumeSelectorArgument(
	arguments []string,
	index int,
	selection *runtimemodel.Selection,
) (int, bool, error) {
	name, value, consumed, ok, err := selectorValue(arguments, index)
	if !ok {
		return index, false, nil
	}
	if err != nil {
		return index, true, err
	}
	if name == "account" {
		if selection.Profile != "" {
			return index, true, errors.New("--account cannot be combined with --profile")
		}
		selection.Account = value
		return index + consumed, true, nil
	}
	if selection.Account != "" {
		return index, true, errors.New("--profile cannot be combined with --account")
	}
	selection.Profile = value
	return index + consumed, true, nil
}

func consumeProviderArgument(
	arguments []string,
	index int,
	selection *runtimemodel.Selection,
) (int, bool, error) {
	name, value, consumed, ok, err := providerValue(arguments, index)
	if !ok {
		return index, false, nil
	}
	if err != nil {
		return index, true, err
	}
	if err := applyProviderSelection(selection, name, value); err != nil {
		return index, true, err
	}
	return index + consumed, true, nil
}

func applyProviderSelection(selection *runtimemodel.Selection, name, value string) error {
	switch name {
	case "provider":
		provider, err := normalizeExternalProvider(value)
		if err != nil {
			return err
		}
		selection.Provider = provider
	case "cli":
		cli, err := normalizeRuntimeCLI(value)
		if err != nil {
			return err
		}
		selection.CLI = cli
	case "api-key":
		selection.APIKey = value
	case "base-url":
		if err := validateProviderBaseURL(value); err != nil {
			return err
		}
		selection.BaseURL = value
	case "url":
		if err := validateCredentialFreeHTTPURL(value, "--url"); err != nil {
			return err
		}
		selection.URL = value
	case providerModelOptionName:
		selection.Model = value
	case providerContextOptionName:
		parsed, err := parseProviderUint(value, "--context-window")
		if err != nil {
			return err
		}
		selection.ContextWindow = &parsed
	case providerCompactOptionName:
		parsed, err := parseProviderUint(value, "--auto-compact-token-limit")
		if err != nil {
			return err
		}
		selection.AutoCompactTokenLimit = &parsed
	}
	return nil
}

func finishRunArguments(
	selection runtimemodel.Selection,
	features runtimeFeatures,
	remaining []string,
) (runtimemodel.Selection, []string, error) {
	if err := validateRunSelection(selection); err != nil {
		return runtimemodel.Selection{}, nil, err
	}
	featureArguments, err := features.arguments()
	if err != nil {
		return runtimemodel.Selection{}, nil, err
	}
	if selection.CLI == "agy" && len(features.nativeOptions) > 0 {
		if slices.Contains(features.nativeOptions, "--presidio") {
			return runtimemodel.Selection{}, nil, errors.New("--presidio is unsupported for native Antigravity")
		}
		return runtimemodel.Selection{}, nil, errors.New("selected options are unsupported for native Antigravity")
	}
	if selection.CLI == "agy" && len(featureArguments) > 0 {
		return runtimemodel.Selection{}, nil, errors.New("selected options are unsupported for native Antigravity")
	}
	codexArguments := append(features.nativeOptions, featureArguments...)
	codexArguments = append(codexArguments, remaining...)
	if selection.CLI == "agy" && codexResumeRequested(codexArguments) {
		return runtimemodel.Selection{}, nil, errors.New("resume is unsupported for native Antigravity")
	}
	if selection.Model != "" && selection.Provider == "" && selection.URL == "" {
		codexArguments = append([]string{"--model", selection.Model}, codexArguments...)
	}
	return selection, codexArguments, nil
}

func validateRunSelection(selection runtimemodel.Selection) error {
	if selection.CLI != "" {
		if selection.CLI != "agy" {
			return fmt.Errorf("invalid --cli: supported values are agy, got %q", selection.CLI)
		}
		if selection.Provider != "gemini" {
			return errors.New("--cli agy requires --provider gemini; use godex run --provider gemini --cli agy")
		}
		if selection.Account != "" || selection.Profile != "" {
			return errors.New("--cli agy cannot use Godex accounts or profiles")
		}
		if selection.APIKey != "" || selection.BaseURL != "" || selection.URL != "" ||
			selection.ContextWindow != nil || selection.AutoCompactTokenLimit != nil || selection.AutoRedeem {
			return errors.New("selected options are unsupported for native Antigravity")
		}
	}
	switch {
	case selection.Provider != "" && selection.Account != "":
		return errors.New("--provider cannot be combined with --account")
	case selection.Provider != "" && selection.URL != "":
		return errors.New("--provider conflicts with --url")
	case selection.BaseURL != "" && selection.URL != "":
		return errors.New("--base-url conflicts with --url")
	case selection.Provider == "" && selection.APIKey != "":
		return errors.New("--api-key requires --provider")
	case selection.Provider == "" && selection.BaseURL != "":
		return errors.New("--base-url requires --provider")
	case (selection.ContextWindow != nil || selection.AutoCompactTokenLimit != nil) &&
		selection.Provider == "" && selection.URL == "":
		return errors.New("context-window options require --provider or --url")
	default:
		return nil
	}
}

func providerValue(arguments []string, index int) (string, string, int, bool, error) {
	if value, consumed, ok, err := namedSecretOptionValue(arguments, index, "--api-key"); ok {
		return "api-key", value, consumed, true, err
	}
	for _, option := range []struct{ flag, name string }{
		{"--provider", "provider"},
		{"--cli", "cli"},
		{"--base-url", "base-url"},
		{"--url", "url"},
		{"--model", providerModelOptionName},
		{"--local-model", providerModelOptionName},
		{"--context-window", providerContextOptionName},
		{"--local-context-window", providerContextOptionName},
		{"--auto-compact-token-limit", providerCompactOptionName},
		{"--local-auto-compact-token-limit", providerCompactOptionName},
	} {
		value, consumed, ok, err := namedOptionValue(arguments, index, option.flag)
		if ok {
			return option.name, value, consumed, true, err
		}
	}
	return "", "", 0, false, nil
}

func namedSecretOptionValue(arguments []string, index int, name string) (string, int, bool, error) {
	argument := arguments[index]
	if argument == name {
		if index+1 >= len(arguments) {
			return "", 0, true, fmt.Errorf(optionRequiresValueFormat, name)
		}
		value := arguments[index+1]
		if value == "" {
			return "", 0, true, fmt.Errorf("%s cannot be empty", name)
		}
		return value, 2, true, nil
	}
	prefix := name + "="
	if strings.HasPrefix(argument, prefix) {
		value := strings.TrimPrefix(argument, prefix)
		if value == "" {
			return "", 0, true, fmt.Errorf("%s cannot be empty", name)
		}
		return value, 1, true, nil
	}
	return "", 0, false, nil
}

func normalizeExternalProvider(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "anthropic", "claude":
		return "anthropic", nil
	case "copilot", "github-copilot", "github_copilot":
		return "copilot", nil
	case "deepseek":
		return "deepseek", nil
	case "gemini":
		return "gemini", nil
	case "kiro":
		return "kiro", nil
	default:
		return "", fmt.Errorf("invalid --provider: supported values are anthropic, copilot, deepseek, gemini, kiro, got %q", strings.ToLower(strings.TrimSpace(value)))
	}
}

func namedOptionValue(arguments []string, index int, name string) (string, int, bool, error) {
	argument := arguments[index]
	if argument == name {
		if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
			return "", 0, true, fmt.Errorf(optionRequiresValueFormat, name)
		}
		return arguments[index+1], 2, true, nil
	}
	prefix := name + "="
	if strings.HasPrefix(argument, prefix) {
		value := strings.TrimPrefix(argument, prefix)
		if strings.TrimSpace(value) == "" {
			return "", 0, true, fmt.Errorf(optionRequiresValueFormat, name)
		}
		return value, 1, true, nil
	}
	return "", 0, false, nil
}

func validateProviderBaseURL(value string) error {
	return validateCredentialFreeHTTPURL(value, "--base-url")
}

func validateCredentialFreeHTTPURL(value, option string) error {
	invalid := func() error {
		return fmt.Errorf("invalid %s: expected an absolute http(s) URL with host and no credentials, query, or fragment", option)
	}
	if strings.HasPrefix(value, "http:///") || strings.HasPrefix(value, "https:///") {
		return invalid()
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		strings.Contains(value, "#") {
		return invalid()
	}
	return nil
}

func parseProviderUint(value, option string) (uint64, error) {
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s expects an unsigned integer", option)
	}
	return parsed, nil
}

func selectorValue(arguments []string, index int) (string, string, int, bool, error) {
	for _, option := range []struct{ flag, name string }{{"--account", "account"}, {"--profile", "profile"}, {"-p", "profile"}} {
		value, consumed, ok, err := namedValue(arguments, index, option.flag)
		if ok {
			return option.name, value, consumed, true, err
		}
	}
	return "", "", 0, false, nil
}

func namedValue(arguments []string, index int, name string) (string, int, bool, error) {
	argument := arguments[index]
	if argument == name {
		if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
			return "", 0, true, fmt.Errorf("%s requires a selector", name)
		}
		return arguments[index+1], 2, true, nil
	}
	prefix := name + "="
	if strings.HasPrefix(argument, prefix) {
		value := strings.TrimSpace(strings.TrimPrefix(argument, prefix))
		if value == "" {
			return "", 0, true, fmt.Errorf("%s requires a selector", name)
		}
		return value, 1, true, nil
	}
	return "", 0, false, nil
}
