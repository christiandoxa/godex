package auth

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	accountmodel "github.com/christiandoxa/godex/internal/model/account"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
)

type LoginOptions struct {
	Name             string
	DeviceAuth       bool
	WithAPIKey       bool
	BaseURL          string
	BaseURLSpecified bool
}

func ParseLoginOptions(arguments []string) (LoginOptions, error) {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "friendly account name")
	deviceAuth := flags.Bool("device-auth", false, "use Codex device authentication")
	withAPIKey := flags.Bool("with-api-key", false, "use an OpenAI/OpenAI-compatible API key")
	baseURL := ""
	baseURLSpecified := false
	setBaseURL := func(value string) error {
		baseURL, baseURLSpecified = value, true
		return nil
	}
	flags.Func("base-url", "OpenAI-compatible base URL", setBaseURL)
	flags.Func("openai-base-url", "OpenAI-compatible base URL", setBaseURL)
	if err := flags.Parse(arguments); err != nil {
		return LoginOptions{}, fmt.Errorf("parse login arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return LoginOptions{}, errors.New("login does not accept positional arguments")
	}
	return LoginOptions{
		Name: *name, DeviceAuth: *deviceAuth, WithAPIKey: *withAPIKey,
		BaseURL: baseURL, BaseURLSpecified: baseURLSpecified,
	}, nil
}

func ShouldPromptLoginMenu(arguments []string) bool {
	options, err := ParseLoginOptions(arguments)
	return err == nil && !options.DeviceAuth && !options.WithAPIKey
}

func Login(ctx context.Context, login *authusecase.Login, out io.Writer, arguments []string) error {
	options, err := ParseLoginOptions(arguments)
	if err != nil {
		return err
	}
	if options.WithAPIKey {
		return errors.New("API-key login requires profile support")
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
