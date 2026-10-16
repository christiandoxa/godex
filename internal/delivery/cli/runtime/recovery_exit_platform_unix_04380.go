//go:build !windows

package runtime

import (
	"errors"
	"os/exec"
	"syscall"
)

func recoveryExitCodeFact04380(code int) int64 { return int64(code) }

func recoveryPlatformSignalFacts04380(err error) (int64, int64, int64) {
	var status *exec.ExitError
	if errors.As(err, &status) {
		if wait, ok := status.ProcessState.Sys().(syscall.WaitStatus); ok && wait.Signaled() {
			return int64(wait.Signal()), int64(syscall.SIGINT), -1
		}
	}
	return -1, int64(syscall.SIGINT), -1
}
