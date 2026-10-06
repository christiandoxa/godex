//go:build windows

package superexpose

import (
	"os/exec"
	"strconv"
	"syscall"
)

const execCreateNewProcessGroup = 0x00000200

func configureExecProcess(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= execCreateNewProcessGroup
}

func stopExecProcessTree(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	killer := exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
	if killer.Run() == nil {
		return
	}
	_ = command.Process.Kill()
}

func execExitSignal(*exec.ExitError) (any, int) { return nil, 1 }
