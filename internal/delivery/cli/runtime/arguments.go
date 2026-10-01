package runtime

import (
	"errors"
	"fmt"
	"strings"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

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

func consumeWrapperArgument(arguments []string, index int, selection *runtimemodel.Selection, features *runtimeFeatures) (int, bool, error) {
	if name, value, consumed, ok, err := selectorValue(arguments, index); ok {
		if err != nil {
			return index, true, err
		}
		if name == "account" {
			if selection.Profile != "" {
				return index, true, errors.New("--account cannot be combined with --profile")
			}
			selection.Account = value
		} else {
			if selection.Account != "" {
				return index, true, errors.New("--profile cannot be combined with --account")
			}
			selection.Profile = value
		}
		return index + consumed, true, nil
	}
	next, handled, err := features.consume(arguments, index)
	return next, handled, err
}

func finishRunArguments(selection runtimemodel.Selection, features runtimeFeatures, remaining []string) (runtimemodel.Selection, []string, error) {
	featureArguments, err := features.arguments()
	if err != nil {
		return runtimemodel.Selection{}, nil, err
	}
	return selection, append(featureArguments, remaining...), nil
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
