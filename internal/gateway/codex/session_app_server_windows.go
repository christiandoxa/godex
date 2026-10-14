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
	arguments []string,
	markerPath string,
	_ func(context.Context, string) error,
) error {
	restoreTerminal := captureOwnedChildTerminal043711(process.terminal.Stdin, process.terminal.Stdout)
	defer restoreTerminal()
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	restoreTerminal()
	printOwnedCompanionResumeGuidance043712(process, markerPath, arguments)
	return nil
}
