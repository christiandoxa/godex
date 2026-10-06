//go:build windows

package subagent

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNewProcessGroup = 0x00000200

func configureProcessGroup(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= createNewProcessGroup
}

func stopProcessTree(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	killer := exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
	if killer.Run() == nil {
		return
	}
	_ = command.Process.Kill()
}

func signalExitCode(*exec.ExitError) int { return 1 }
