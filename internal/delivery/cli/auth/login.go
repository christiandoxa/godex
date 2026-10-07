package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	accountmodel "github.com/christiandoxa/godex/internal/model/account"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
)

type LoginOptions struct {
	Name             string
	Profile          string
	Status           bool
	DeviceAuth       bool
	WithAPIKey       bool
	WithAntigravity  bool
	BaseURL          string
	BaseURLSpecified bool
}

func ParseLoginOptions(arguments []string) (LoginOptions, error) {
	options := LoginOptions{}
	positionalProfile := ""
	requestedAntigravity := false
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--device-auth":
			options.DeviceAuth = true
		case argument == "--with-api-key":
			options.WithAPIKey = true
		case argument == "--with-antigravity" || argument == "--antigravity" ||
			argument == "--with-agy" || argument == "--agy":
			requestedAntigravity = true
		case argument == "status":
			options.Status = true
		case argument == "--":
			for _, value := range arguments[index+1:] {
				if value == "status" && !options.Status {
					options.Status = true
					continue
				}
				if positionalProfile == "" && !strings.HasPrefix(value, "-") {
					positionalProfile = value
					continue
				}
				return LoginOptions{}, errors.New("login accepts at most one profile and optional status")
			}
			index = len(arguments)
		case argument == "--name" || strings.HasPrefix(argument, "--name="):
			value, next, err := loginOptionValue(arguments, index, "--name")
			if err != nil {
				return LoginOptions{}, err
			}
			options.Name, index = value, next
		case argument == "--profile" || strings.HasPrefix(argument, "--profile=") ||
			argument == "-p" || strings.HasPrefix(argument, "-p="):
			value, next, err := loginOptionValue(arguments, index, "--profile", "-p")
			if err != nil {
				return LoginOptions{}, err
			}
			if options.Profile != "" {
				return LoginOptions{}, errors.New("login accepts at most one --profile")
			}
			options.Profile, index = value, next
		case argument == "--base-url" || strings.HasPrefix(argument, "--base-url=") ||
			argument == "--openai-base-url" || strings.HasPrefix(argument, "--openai-base-url="):
			value, next, err := loginOptionValueAllowEmpty(arguments, index, "--base-url", "--openai-base-url")
			if err != nil {
				return LoginOptions{}, err
			}
			options.BaseURL, options.BaseURLSpecified, index = value, true, next
		case strings.HasPrefix(argument, "-"):
			return LoginOptions{}, fmt.Errorf("unknown login option %q", argument)
		default:
			if positionalProfile != "" {
				return LoginOptions{}, errors.New("login accepts at most one positional profile")
			}
			positionalProfile = argument
		}
	}
	if positionalProfile != "" {
		if options.Profile != "" && options.Profile != positionalProfile {
			return LoginOptions{}, errors.New("positional profile conflicts with --profile")
		}
		options.Profile = positionalProfile
	}
	if options.Profile != "" && options.Name != "" {
		return LoginOptions{}, errors.New("--profile cannot be combined with --name")
	}
	options.WithAntigravity = requestedAntigravity && !options.WithAPIKey
	if options.WithAntigravity {
		if options.BaseURLSpecified {
			return LoginOptions{}, errors.New("--base-url is not supported for Antigravity login")
		}
		if options.Name != "" {
			return LoginOptions{}, errors.New("--name is not supported for Antigravity login")
		}
		if options.Profile != "" {
			return LoginOptions{}, errors.New("Antigravity login is global and does not use Godex profiles")
		}
	}
	return options, nil
}

func loginOptionValueAllowEmpty(arguments []string, index int, names ...string) (string, int, error) {
	argument := arguments[index]
	for _, name := range names {
		if argument == name {
			if index+1 >= len(arguments) {
				return "", index, fmt.Errorf("%s requires a value", name)
			}
			return arguments[index+1], index + 1, nil
		}
		prefix := name + "="
		if strings.HasPrefix(argument, prefix) {
			return strings.TrimPrefix(argument, prefix), index, nil
		}
	}
	return "", index, errors.New("login option requires a value")
}

func loginOptionValue(arguments []string, index int, names ...string) (string, int, error) {
	argument := arguments[index]
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
	return "", index, errors.New("login option requires a value")
}

func ShouldPromptLoginMenu(arguments []string) bool {
	options, err := ParseLoginOptions(arguments)
	return err == nil && !options.DeviceAuth && !options.WithAPIKey && !options.WithAntigravity
}

func Login(ctx context.Context, login *authusecase.Login, native *authusecase.Native, out io.Writer, arguments []string) error {
	options, err := ParseLoginOptions(arguments)
	if err != nil {
		return err
	}
	if options.WithAPIKey {
		return errors.New("API-key login requires profile support")
	}
	if options.WithAntigravity {
		if native == nil {
			return errors.New("native authentication commands are not configured")
		}
		return native.RunAntigravityLogin(ctx)
	}
	if options.BaseURLSpecified {
		return errors.New("--base-url is only supported for API key login")
	}
	account, err := login.Run(ctx, accountmodel.LoginInput{Name: options.Name, DeviceAuth: options.DeviceAuth})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Logged in as %s (%s).\n", account.Name, displayIdentity(account.Email, account.ID))
	return err
}

func displayIdentity(email, fallback string) string {
	if strings.TrimSpace(email) != "" {
		return email
	}
	return fallback
}
