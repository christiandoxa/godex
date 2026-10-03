//go:build !windows

package main

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
)

func TestExitCodePreservesChildSignalStatus(t *testing.T) {
	err := exec.Command("sh", "-c", "kill -TERM $$").Run()
	if got, want := exitCode(context.Background(), err), 128+int(syscall.SIGTERM); got != want {
		t.Fatalf("signaled child exit code = %d, want %d", got, want)
	}
}
