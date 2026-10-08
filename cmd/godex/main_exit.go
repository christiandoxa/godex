package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func exitCode(ctx context.Context, err error) int {
	var childError *exec.ExitError
	if errors.As(err, &childError) {
		if code := childError.ExitCode(); code >= 0 {
			return code
		}
		if childError.ProcessState != nil {
			if status, ok := childError.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return 128 + int(status.Signal())
			}
		}
	}
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		if code := coded.ExitCode(); code >= 0 {
			return code
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return 130
	}
	_, _ = fmt.Fprintln(os.Stderr, errorPrefix, err)
	return 1
}
