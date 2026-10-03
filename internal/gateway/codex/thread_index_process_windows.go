//go:build windows

package codex

import (
	"os/exec"
	"strconv"
	"syscall"
)

const codexAppServerNewProcessGroup = 0x00000200

func configureCodexAppServerProcess(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= codexAppServerNewProcessGroup
}

func terminateCodexAppServerProcess(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	killer := exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
	if killer.Run() == nil {
		return
	}
	_ = command.Process.Kill()
}
