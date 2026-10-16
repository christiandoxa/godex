//go:build !windows

package codex

import (
	"context"
	"os/exec"
)

func configureTerminalCommandContext(_ *exec.Cmd) {}

func runTerminalCommand(ctx context.Context, command *exec.Cmd) error {
	privateProcessGroup := configureCodexSessionChildProcess(command)
	prepareTerminalCommandWait(command)
	if err := command.Start(); err != nil {
		return err
	}
	return waitForSessionChild(ctx, command, privateProcessGroup)
}

// waitForSessionChild owns cancellation so interactive SIGINT can reach the
// child before the second interrupt requests teardown.
func prepareTerminalCommandWait(command *exec.Cmd) {
	command.Cancel = nil
	command.WaitDelay = 0
}
