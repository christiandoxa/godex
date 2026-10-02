//go:build windows

package kiro

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNewProcessGroup = 0x00000200

func configureACPProcess(command *exec.Cmd, ownGroup bool) {
	if command == nil || !ownGroup {
		return
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= createNewProcessGroup
}

func terminateACPProcess(command *exec.Cmd, ownGroup bool) {
	if command == nil || command.Process == nil {
		return
	}
	if ownGroup {
		killer := exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
		if killer.Run() == nil {
			return
		}
	}
	_ = command.Process.Kill()
}
