package runtime

import (
	"errors"
	"strings"
)

func parseRunArguments(arguments []string) (string, []string, error) {
	selector := ""
	features := runtimeFeatures{}
	for index := 0; index < len(arguments); {
		if arguments[index] == "--" {
			return finishRunArguments(selector, features, arguments[index+1:])
		}
		value, next, handled, err := consumeWrapperArgument(arguments, index, &features)
		if err != nil {
			return "", nil, err
		}
		if !handled {
			return finishRunArguments(selector, features, arguments[index:])
		}
		if value != "" {
			selector = value
		}
		index = next
	}
	return finishRunArguments(selector, features, nil)
}

func consumeWrapperArgument(arguments []string, index int, features *runtimeFeatures) (string, int, bool, error) {
	if value, consumed, ok, err := accountValue(arguments, index); ok {
		return value, index + consumed, true, err
	}
	next, handled, err := features.consume(arguments, index)
	return "", next, handled, err
}

func finishRunArguments(selector string, features runtimeFeatures, remaining []string) (string, []string, error) {
	featureArguments, err := features.arguments()
	if err != nil {
		return "", nil, err
	}
	return selector, append(featureArguments, remaining...), nil
}

func accountValue(arguments []string, index int) (string, int, bool, error) {
	argument := arguments[index]
	if argument == "--account" {
		if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
			return "", 0, true, errors.New("--account requires a selector")
		}
		return arguments[index+1], 2, true, nil
	}
	const prefix = "--account="
	if strings.HasPrefix(argument, prefix) {
		value := strings.TrimSpace(strings.TrimPrefix(argument, prefix))
		if value == "" {
			return "", 0, true, errors.New("--account requires a selector")
		}
		return value, 1, true, nil
	}
	return "", 0, false, nil
}
