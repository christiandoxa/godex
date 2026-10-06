//go:build unix && !linux

package superexpose

import (
	"errors"
	"os/exec"
	"syscall"
)

func configureExecProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopExecProcessTree(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = command.Process.Kill()
	}
}

func execExitSignal(exitErr *exec.ExitError) (any, int) {
	if exitErr != nil && exitErr.ProcessState != nil {
		if status, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			signal := int(status.Signal())
			return signal, 128 + signal
		}
	}
	return nil, 1
}
