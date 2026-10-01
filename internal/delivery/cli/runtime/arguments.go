package runtime

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const optionRequiresValueFormat = "%s requires a value"

func parseRunArguments(arguments []string) (runtimemodel.Selection, []string, error) {
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
	case "api-key":
		selection.APIKey = value
	case "base-url":
		if err := validateProviderBaseURL(value); err != nil {
			return err
		}
		selection.BaseURL = value
	}
	return nil
}

func finishRunArguments(selection runtimemodel.Selection, features runtimeFeatures, remaining []string) (runtimemodel.Selection, []string, error) {
	if selection.Provider != "" && selection.Account != "" {
		return runtimemodel.Selection{}, nil, errors.New("--provider cannot be combined with --account")
	}
	if selection.Provider == "" && selection.APIKey != "" {
		return runtimemodel.Selection{}, nil, errors.New("--api-key requires --provider")
	}
	if selection.Provider == "" && selection.BaseURL != "" {
		return runtimemodel.Selection{}, nil, errors.New("--base-url requires --provider")
	}
	featureArguments, err := features.arguments()
	if err != nil {
		return runtimemodel.Selection{}, nil, err
	}
	return selection, append(featureArguments, remaining...), nil
}

func providerValue(arguments []string, index int) (string, string, int, bool, error) {
	if value, consumed, ok, err := namedSecretOptionValue(arguments, index, "--api-key"); ok {
		return "api-key", value, consumed, true, err
	}
	for _, option := range []struct{ flag, name string }{
		{"--provider", "provider"},
		{"--base-url", "base-url"},
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
	invalid := func() error {
		return errors.New("invalid --base-url: expected an absolute http(s) URL with host and no credentials, query, or fragment")
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
