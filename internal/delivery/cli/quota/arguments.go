package quota

import (
	"errors"
	"fmt"
	"strings"

	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

type showOptions struct {
	quotausecase.Options
	detail bool
	raw    bool
	once   bool
	watch  bool
}

func parseArguments(arguments []string) (showOptions, error) {
	options := showOptions{}
	for index := 0; index < len(arguments); index++ {
		next, err := consumeQuotaArgument(arguments, index, &options)
		if err != nil {
			return showOptions{}, err
		}
		index = next
	}
	if err := options.validate(); err != nil {
		return showOptions{}, err
	}
	return options, nil
}

func consumeQuotaArgument(arguments []string, index int, options *showOptions) (int, error) {
	argument := arguments[index]
	switch argument {
	case "--all":
		options.All = true
	case "--detail":
		options.detail = true
	case "--raw":
		options.raw = true
	case "--once":
		options.once = true
	case "--watch":
		options.watch = true
	case "--help", "-h":
		return index, errors.New(quotaUsage)
	default:
		return consumeQuotaValueArgument(arguments, index, options)
	}
	return index, nil
}

func consumeQuotaValueArgument(arguments []string, index int, options *showOptions) (int, error) {
	argument := arguments[index]
	if quotaProfileOption(argument) {
		return consumeQuotaProfile(arguments, index, options)
	}
	if argument == "--base-url" || strings.HasPrefix(argument, "--base-url=") {
		value, next, err := quotaOptionValue(arguments, index, argument, "--base-url")
		if err != nil {
			return index, err
		}
		options.BaseURL = value
		return next, nil
	}
	if quotaAuthOption(argument) {
		return consumeQuotaAuth(arguments, index, options)
	}
	if quotaProviderOption(argument) {
		return consumeQuotaProvider(arguments, index, options)
	}
	if strings.HasPrefix(argument, "-") {
		return index, fmt.Errorf("unknown quota option %q", argument)
	}
	if options.Selector != "" {
		return index, errors.New("quota accepts at most one profile selector")
	}
	options.Selector = argument
	return index, nil
}

func quotaAuthOption(argument string) bool {
	return argument == "--auth" || strings.HasPrefix(argument, "--auth=")
}

func consumeQuotaAuth(arguments []string, index int, options *showOptions) (int, error) {
	value, next, err := quotaOptionValue(arguments, index, arguments[index], "--auth")
	if err != nil {
		return index, err
	}
	options.AuthFilter = strings.ToLower(strings.TrimSpace(value))
	return next, nil
}

func quotaProviderOption(argument string) bool {
	return argument == "--provider" || strings.HasPrefix(argument, "--provider=")
}

func consumeQuotaProvider(arguments []string, index int, options *showOptions) (int, error) {
	value, next, err := quotaOptionValue(arguments, index, arguments[index], "--provider")
	if err != nil {
		return index, err
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if !validQuotaProviderFilter(value) {
		return index, fmt.Errorf("unsupported quota provider filter %q", value)
	}
	options.ProviderFilter = value
	return next, nil
}

func quotaProfileOption(argument string) bool {
	return argument == "--profile" || argument == "-p" || strings.HasPrefix(argument, "--profile=") || strings.HasPrefix(argument, "-p=")
}

func consumeQuotaProfile(arguments []string, index int, options *showOptions) (int, error) {
	value, next, err := quotaOptionValue(arguments, index, arguments[index], "--profile", "-p")
	if err != nil {
		return index, err
	}
	if options.Selector != "" {
		return index, errors.New("quota accepts at most one profile selector")
	}
	options.Selector = value
	return next, nil
}

func (options showOptions) validate() error {
	if options.All && options.Selector != "" {
		return errors.New("quota profile selector cannot be combined with --all")
	}
	if options.raw && (options.All || options.detail || options.watch || options.once || options.AuthFilter != "" || options.ProviderFilter != "") {
		return errors.New("quota --raw cannot be combined with --all, --detail, --watch, --once, --auth, or --provider")
	}
	if options.Selector != "" && (options.AuthFilter != "" || options.ProviderFilter != "") {
		return errors.New("quota profile selector cannot be combined with --auth or --provider")
	}
	if options.watch && options.once {
		return errors.New("quota --watch cannot be combined with --once")
	}
	return nil
}

func (options showOptions) watchEnabled() bool {
	return !options.raw && !options.once
}

func validQuotaProviderFilter(value string) bool {
	switch value {
	case "all", "openai", "gemini", "anthropic", "claude", "copilot", "kiro", "deepseek", "local", "agy":
		return true
	default:
		return false
	}
}

func quotaOptionValue(arguments []string, index int, argument string, names ...string) (string, int, error) {
	for _, name := range names {
		if argument == name {
			if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
				return "", index, fmt.Errorf("%s requires a value", name)
			}
			return arguments[index+1], index + 1, nil
		}
		prefix := name + "="
		if strings.HasPrefix(argument, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(argument, prefix))
			if value == "" {
				return "", index, fmt.Errorf("%s requires a value", name)
			}
			return value, index, nil
		}
	}
	return "", index, errors.New("quota option requires a value")
}

const quotaUsage = "usage: godex quota [-p NAME|selector] [--all] [--auth AUTH] [--provider PROVIDER] [--detail] [--raw] [--once|--watch] [--base-url URL]"
