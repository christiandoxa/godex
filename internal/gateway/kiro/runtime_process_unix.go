//go:build !windows

package kiro

import (
	"os/exec"
	"syscall"
)

func configureACPProcess(command *exec.Cmd, ownGroup bool) {
	if command == nil || !ownGroup {
		return
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
}

func terminateACPProcess(command *exec.Cmd, ownGroup bool) {
	if command == nil || command.Process == nil {
		return
	}
	if ownGroup {
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err == nil {
			return
		}
	}
	_ = command.Process.Kill()
}
