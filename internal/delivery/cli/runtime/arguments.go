package runtime

import (
	"errors"
	"strings"
)

func parseRunArguments(arguments []string) (string, []string, error) {
	selector := ""
	features := runtimeFeatures{}
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if argument == "--" {
			featureArguments, err := features.arguments()
			if err != nil {
				return "", nil, err
			}
			return selector, append(featureArguments, arguments[index+1:]...), nil
		}
		if value, consumed, ok, err := accountValue(arguments, index); ok {
			if err != nil {
				return "", nil, err
			}
			selector = value
			index += consumed
			continue
		}
		next, handled, err := features.consume(arguments, index)
		if err != nil {
			return "", nil, err
		}
		if handled {
			index = next
			continue
		}
		featureArguments, err := features.arguments()
		if err != nil {
			return "", nil, err
		}
		return selector, append(featureArguments, arguments[index:]...), nil
	}
	featureArguments, err := features.arguments()
	return selector, featureArguments, err
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
