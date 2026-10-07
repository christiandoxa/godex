//go:build windows

package superexpose

import (
	"context"
	"os/exec"
	"strconv"
	"syscall"
	"time"
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	killer := exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
	if killer.Run() == nil {
		return
	}
	_ = command.Process.Kill()
}

func execExitSignal(*exec.ExitError) (any, int) { return nil, 1 }
