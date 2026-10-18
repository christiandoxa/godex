//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
)

func configureGatewayProcess(*exec.Cmd)     {}
func signalGateway(command *exec.Cmd) error { return command.Process.Signal(os.Interrupt) }
func stopNativeGateway([]string) error      { return errors.New("native console control is Windows-only") }
