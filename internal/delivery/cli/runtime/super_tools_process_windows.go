//go:build windows

package runtime

import (
	"context"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const superProbeCreateNewProcessGroup = 0x00000200

func configureSuperProbeProcess(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= superProbeCreateNewProcessGroup
}

func stopSuperProbeProcessTree(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	killer := exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
	if killer.Run() == nil {
		return
	}
	_ = command.Process.Kill()
}
