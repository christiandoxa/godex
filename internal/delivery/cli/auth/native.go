package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	authmodel "github.com/christiandoxa/godex/internal/model/auth"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
)

func Native(ctx context.Context, native *authusecase.Native, logout bool, args []string) error {
	if native == nil {
		return errors.New("native authentication commands are not configured")
	}
	selector := ""
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--account":
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
				return errors.New("--account requires a value")
			}
			if selector != "" && selector != args[index+1] {
				return errors.New("conflicting authentication selectors")
			}
			selector = args[index+1]
			index++
		case strings.HasPrefix(argument, "--account="):
			value := strings.TrimSpace(strings.TrimPrefix(argument, "--account="))
			if value == "" {
				return errors.New("--account requires a value")
			}
			if selector != "" && selector != value {
				return errors.New("conflicting authentication selectors")
			}
			selector = value
		case strings.HasPrefix(argument, "-"):
			return fmt.Errorf("unknown authentication option %q", argument)
		default:
			return errors.New("usage: godex login status [--account SELECTOR]")
		}
	}
	return native.Run(ctx, authmodel.Command{Selector: selector, Logout: logout})
}

func ParseLogoutSelector(args []string) (string, error) {
	selector := ""
	for index := 0; index < len(args); index++ {
		argument := args[index]
		var value string
		switch {
		case argument == "-p" || argument == "--profile" || argument == "--account":
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
				return "", fmt.Errorf("%s requires a value", argument)
			}
			value = strings.TrimSpace(args[index+1])
			index++
		case strings.HasPrefix(argument, "--profile="):
			value = strings.TrimSpace(strings.TrimPrefix(argument, "--profile="))
		case strings.HasPrefix(argument, "-p="):
			value = strings.TrimSpace(strings.TrimPrefix(argument, "-p="))
		case strings.HasPrefix(argument, "--account="):
			value = strings.TrimSpace(strings.TrimPrefix(argument, "--account="))
		case strings.HasPrefix(argument, "-"):
			return "", fmt.Errorf("unknown logout option %q", argument)
		default:
			value = strings.TrimSpace(argument)
		}
		if value == "" {
			return "", errors.New("logout profile selector cannot be empty")
		}
		if selector != "" && selector != value {
			return "", errors.New("logout accepts only one profile selector")
		}
		selector = value
	}
	return selector, nil
}
