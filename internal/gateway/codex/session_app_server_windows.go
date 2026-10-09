//go:build windows

package codex

import (
	"context"
	"os/exec"
)

func (process *CodexProcess) runWithSessionAppServer(
	ctx context.Context,
	_ string,
	_ string,
	_ []string,
	command *exec.Cmd,
	_ []string,
) error {
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}
