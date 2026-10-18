package main

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureGatewayProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE, HideWindow: true}
}

func signalGateway(command *exec.Cmd) error {
	helper, err := os.Executable()
	if err != nil {
		return err
	}
	return exec.Command(helper, "__native-stop", strconv.Itoa(command.Process.Pid)).Run()
}

// Attach a short-lived sender to the child's private console. Broadcasting
// CTRL_C there cannot interrupt the CI runner or another oracle process.
func stopNativeGateway(arguments []string) error {
	if len(arguments) != 1 {
		return errors.New("native console stop requires one PID")
	}
	pid, err := strconv.ParseUint(arguments[0], 10, 32)
	if err != nil || pid == 0 {
		return errors.New("native console stop requires a positive PID")
	}
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	_, _, _ = kernel.NewProc("FreeConsole").Call()
	ok, _, callErr := kernel.NewProc("AttachConsole").Call(uintptr(pid))
	if ok == 0 {
		return callErr
	}
	defer kernel.NewProc("FreeConsole").Call()
	ok, _, callErr = kernel.NewProc("SetConsoleCtrlHandler").Call(0, 1)
	if ok == 0 {
		return callErr
	}
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, 0)
}
