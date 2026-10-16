//go:build windows

package codex

import (
	"context"
	"os/exec"
)

func runTerminalCommand(_ context.Context, command *exec.Cmd) error {
	return command.Run()
}
