//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type commandResult struct {
	output   []byte
	exitCode int
}

func runCommand(ctx context.Context, binary string, arguments []string, environment []string, tty bool) (commandResult, error) {
	if !tty {
		var output bytes.Buffer
		command := exec.CommandContext(ctx, binary, arguments...)
		command.Env = environment
		command.Stdin = strings.NewReader("")
		command.Stdout = &output
		command.Stderr = &output
		err := command.Run()
		return commandResult{output: output.Bytes(), exitCode: commandExitCode(err)}, err
	}

	master, slave, err := openPTY()
	if err != nil {
		return commandResult{}, err
	}
	defer master.Close()
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Env = environment
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Stdin, command.Stdout, command.Stderr = slave, slave, slave
	if err := command.Start(); err != nil {
		_ = slave.Close()
		return commandResult{}, err
	}
	_ = slave.Close()
	output := make(chan []byte, 1)
	go func() { output <- readPTY(master) }()
	err = command.Wait()
	result := <-output
	_ = master.Close()
	return commandResult{output: result, exitCode: commandExitCode(err)}, err
}

func openPTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open PTY master: %w", err)
	}
	closeMaster := func() { _ = master.Close() }
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		closeMaster()
		return nil, nil, fmt.Errorf("unlock PTY: %w", err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		closeMaster()
		return nil, nil, fmt.Errorf("inspect PTY number: %w", err)
	}
	slave, err := os.OpenFile(filepath.Join("/dev/pts", fmt.Sprint(number)), os.O_RDWR, 0)
	if err != nil {
		closeMaster()
		return nil, nil, fmt.Errorf("open PTY slave: %w", err)
	}
	return master, slave, nil
}

func readPTY(master *os.File) []byte {
	var output bytes.Buffer
	buffer := make([]byte, 4096)
	for {
		count, err := master.Read(buffer)
		if count > 0 {
			_, _ = output.Write(buffer[:count])
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) {
				break
			}
			break
		}
	}
	return output.Bytes()
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
			if status.Exited() {
				return status.ExitStatus()
			}
			if status.Signaled() {
				return 128 + int(status.Signal())
			}
		}
		return exitError.ExitCode()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 124
	}
	return 1
}

func commandContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 90*time.Second)
}
