//go:build windows

package codex

import (
	"context"
	"os"
	"os/exec"
	"time"

	"golang.org/x/term"
)

func configureTerminalCommandContext(command *exec.Cmd) {
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		if file, ok := command.Stdin.(*os.File); ok && file != nil && term.IsTerminal(int(file.Fd())) {
			return nil
		}
		if err := command.Process.Signal(os.Interrupt); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
	command.WaitDelay = 2 * time.Second
}

func runTerminalCommand(_ context.Context, command *exec.Cmd) error {
	return command.Run()
}
