//go:build !unix && !windows

package runtimebroker

import "os"

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	return err == nil && process != nil
}
