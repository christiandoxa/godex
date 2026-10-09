//go:build linux

package account

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// processAlive treats a zombie as absent. kill(pid, 0) reports a zombie as
// present even though its owner has exited and cannot release a directory
// lock. The /proc state check closes that crash-recovery gap while retaining
// the conservative behavior for processes we cannot inspect.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	if err != nil {
		return true
	}
	return !linuxProcessZombie(pid)
}

func linuxProcessZombie(pid int) bool {
	content, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	closeParen := strings.LastIndexByte(string(content), ')')
	return closeParen >= 0 && closeParen+2 < len(content) && content[closeParen+2] == 'Z'
}
