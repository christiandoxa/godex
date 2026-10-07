package codex

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

// LoginArguments runs the official Codex login command with passthrough login
// arguments after Godex has resolved its own profile selector.
func (process *CodexProcess) LoginArguments(
	ctx context.Context,
	codexHome string,
	arguments []string,
) (entity.Identity, error) {
	binary, err := process.resolveBinary()
	if err != nil {
		return entity.Identity{}, err
	}
	if err := prepareCodexHome(codexHome); err != nil {
		return entity.Identity{}, err
	}
	release, err := (SessionLocker{}).LockCodexSessionsForChild(ctx, codexHome)
	if err != nil {
		return entity.Identity{}, err
	}
	defer release()

	commandArgs := append([]string{"login"}, arguments...)
	command := exec.CommandContext(ctx, binary, commandArgs...)
	command.Env = environmentWith("CODEX_HOME", codexHome)
	command.Stdin = process.terminal.Stdin
	command.Stdout = process.terminal.Stdout
	command.Stderr = process.terminal.Stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return entity.Identity{}, ctx.Err()
		}
		return entity.Identity{}, fmt.Errorf("codex login failed: %w", err)
	}
	if err := secureCodexHome(codexHome); err != nil {
		return entity.Identity{}, err
	}
	identity, err := readChatGPTIdentity(filepath.Join(codexHome, "auth.json"))
	if err != nil {
		return entity.Identity{}, err
	}
	return identity, nil
}
