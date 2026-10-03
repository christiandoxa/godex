//go:build !windows

package codex

import (
	"os/exec"
	"syscall"
)

func configureCodexAppServerProcess(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
}

func terminateCodexAppServerProcess(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err == nil {
		return
	}
	_ = command.Process.Kill()
}
