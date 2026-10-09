//go:build linux

package runtimebroker

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// processAlive mirrors Prodex's Linux absence proof: a zombie has exited even
// though kill(pid, 0) still reports that its unreaped process entry exists.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	state, err := linuxProcessState(pid)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	return state != "Z"
}

func linuxProcessState(pid int) (string, error) {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	return linuxProcessStateFromStat(string(stat))
}

func linuxProcessStateFromStat(stat string) (string, error) {
	fields, err := linuxProcessFields(stat)
	if err != nil {
		return "", err
	}
	return fields[0], nil
}

func linuxProcessFields(stat string) ([]string, error) {
	separator := strings.LastIndex(stat, ") ")
	if separator < 0 {
		return nil, errors.New("invalid Linux process stat")
	}
	fields := strings.Fields(stat[separator+2:])
	if len(fields) == 0 {
		return nil, errors.New("missing Linux process state")
	}
	return fields, nil
}
