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
	WithClaude       bool
	WithAntigravity  bool
	WithAccessToken  bool
	BaseURL          string
	BaseURLSpecified bool
	CodexArgs        []string
}

const geminiOAuthDisabledGuidance = "Google Gemini OAuth profiles are unsupported and disabled. For Codex-fronted Gemini, migrate to a Gemini API key (`--api-key`, `GEMINI_API_KEY`, or `GOOGLE_API_KEY`). Native Gemini CLI / Vertex AI compatibility was retired; use the Gemini provider bridge with API-key authentication."

func ParseLoginOptions(arguments []string) (LoginOptions, error) {
	options := LoginOptions{}
	index := 0
	for index < len(arguments) {
		argument := arguments[index]
		switch {
		case argument == "--profile" || strings.HasPrefix(argument, "--profile=") ||
			argument == "-p" || strings.HasPrefix(argument, "-p=") ||
			argument == "--account" || strings.HasPrefix(argument, "--account="):
			value, next, err := loginOptionValue(arguments, index, "--profile", "-p", "--account")
			if err != nil {
				return LoginOptions{}, err
			}
			if options.Profile != "" && options.Profile != value {
				return LoginOptions{}, errors.New("login accepts at most one --profile")
			}
			options.Profile, index = value, next+1
		default:
			index = len(arguments)
		}
	}

	consumed := 0
	for consumed < len(arguments) {
		argument := arguments[consumed]
		if argument == "--profile" || argument == "-p" || argument == "--account" {
			consumed += 2
			continue
		}
		if strings.HasPrefix(argument, "--profile=") || strings.HasPrefix(argument, "-p=") ||
			strings.HasPrefix(argument, "--account=") {
			consumed++
			continue
		}
		break
	}
	methodArgs := append([]string(nil), arguments[consumed:]...)
	var err error
	methodArgs, options.Name, err = extractLoginCompatName(methodArgs, options.Name)
	if err != nil {
		return LoginOptions{}, err
	}
	if options.Profile == "" && len(methodArgs) > 0 &&
		!strings.HasPrefix(methodArgs[0], "-") && methodArgs[0] != "status" {
		options.Profile = methodArgs[0]
		methodArgs = methodArgs[1:]
	}
	if options.Profile != "" && options.Name != "" {
		return LoginOptions{}, errors.New("--profile cannot be combined with --name")
	}

	filtered, baseURL, baseURLSpecified, err := extractLoginBaseURL(methodArgs)
	if err != nil {
		return LoginOptions{}, err
	}
	options.CodexArgs = filtered
	options.BaseURL, options.BaseURLSpecified = baseURL, baseURLSpecified

	if len(filtered) > 0 && filtered[0] == "status" {
		options.Status = true
		if options.BaseURLSpecified {
			return LoginOptions{}, errors.New("--base-url is only supported for API key login")
		}
		return options, nil
	}

	hasAPIKey, hasClaude, hasAntigravity := false, false, false
	hasAccessToken, hasDevice, removedGemini := false, false, false
	for _, argument := range filtered {
		switch argument {
		case "--with-api-key":
			hasAPIKey = true
		case "--with-claude", "--claude":
			hasClaude = true
		case "--with-antigravity", "--antigravity", "--with-agy", "--agy":
			hasAntigravity = true
		case "--with-access-token":
			hasAccessToken = true
		case "--device-auth":
			hasDevice = true
		case "--with-google", "--google":
			removedGemini = true
		}
	}
	if removedGemini {
		return LoginOptions{}, errors.New(geminiOAuthDisabledGuidance)
	}
	switch {
	case hasAPIKey:
		options.WithAPIKey = true
	case hasClaude:
		options.WithClaude = true
	case hasAntigravity:
		options.WithAntigravity = true
	case hasAccessToken:
		options.WithAccessToken = true
	case hasDevice:
		options.DeviceAuth = true
	}
	if options.BaseURLSpecified && (options.WithClaude || options.WithAntigravity || options.WithAccessToken || options.DeviceAuth) {
		if options.WithAntigravity {
			return LoginOptions{}, errors.New("--base-url is not supported for Antigravity login")
		}
		return LoginOptions{}, errors.New("--base-url is only supported for API key login")
	}
	if options.WithAntigravity {
		if options.Name != "" {
			return LoginOptions{}, errors.New("--name is not supported for Antigravity login")
		}
		if options.Profile != "" {
			return LoginOptions{}, errors.New("Antigravity login is global and does not use Godex profiles")
		}
	}
	return options, nil
}

func extractLoginCompatName(arguments []string, initial string) ([]string, string, error) {
	filtered := make([]string, 0, len(arguments))
	name := initial
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--name":
			if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
				return nil, "", errors.New("--name requires a value")
			}
			name = arguments[index+1]
			index++
		case strings.HasPrefix(argument, "--name="):
			value := strings.TrimSpace(strings.TrimPrefix(argument, "--name="))
			if value == "" {
				return nil, "", errors.New("--name requires a value")
			}
			name = value
		default:
			filtered = append(filtered, argument)
		}
	}
	return filtered, name, nil
}

func extractLoginBaseURL(arguments []string) ([]string, string, bool, error) {
	filtered := make([]string, 0, len(arguments))
	baseURL := ""
	specified := false
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--base-url" || argument == "--openai-base-url":
			if index+1 >= len(arguments) {
				return nil, "", false, errors.New("--base-url requires a URL value")
			}
			baseURL, specified = arguments[index+1], true
			index++
		case strings.HasPrefix(argument, "--base-url="):
			baseURL, specified = strings.TrimPrefix(argument, "--base-url="), true
		case strings.HasPrefix(argument, "--openai-base-url="):
			baseURL, specified = strings.TrimPrefix(argument, "--openai-base-url="), true
		default:
			filtered = append(filtered, argument)
		}
	}
	return filtered, baseURL, specified, nil
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
	return err == nil && !options.Status && !options.DeviceAuth && !options.WithAPIKey &&
		!options.WithClaude && !options.WithAntigravity && !options.WithAccessToken && len(options.CodexArgs) == 0
}

func Login(ctx context.Context, login *authusecase.Login, native *authusecase.Native, out io.Writer, arguments []string) error {
	options, err := ParseLoginOptions(arguments)
	if err != nil {
		return err
	}
	if options.WithAPIKey {
		return errors.New("API-key login requires profile support")
	}
	if options.WithClaude {
		return errors.New("Claude login requires profile support")
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
	account, err := login.RunArguments(
		ctx,
		accountmodel.LoginInput{Name: options.Name, DeviceAuth: options.DeviceAuth},
		options.CodexArgs,
	)
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
