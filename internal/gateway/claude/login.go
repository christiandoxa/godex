package claude

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

type oauthLoginRunner func(context.Context, string, []string, []string) error

func (source *Source) LoginOAuth(
	ctx context.Context,
	configDir string,
	email string,
) (profilemodel.BuiltinCredential, error) {
	if err := ctx.Err(); err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	configDir, err := validateConfigDir(configDir)
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	args := []string{"auth", "login", "--claudeai"}
	if email = strings.TrimSpace(email); email != "" {
		args = append(args, "--email", email)
	}
	environment := claudeRuntimeEnvironment(configDir)
	if source.oauthLogin != nil {
		if err := source.oauthLogin(ctx, configDir, args, environment); err != nil {
			return profilemodel.BuiltinCredential{}, err
		}
	} else {
		binary := strings.TrimSpace(source.getenv("CLAUDE_BIN"))
		if binary == "" {
			binary = "claude"
		}
		command := exec.CommandContext(ctx, binary, args...)
		command.Env = environment
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			if ctx.Err() != nil {
				return profilemodel.BuiltinCredential{}, ctx.Err()
			}
			return profilemodel.BuiltinCredential{}, errors.New("Claude OAuth login failed")
		}
	}
	text, err := readExternalCredential(configDir)
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	return source.InspectCredential(ctx, text)
}
