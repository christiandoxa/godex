//go:build linux

package runtimebroker

import (
	"fmt"
	"os"
	"strings"
)

func processBirthIdentity(pid uint32) string {
	if pid == 0 {
		return ""
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	fields, err := linuxProcessFields(string(stat))
	if err != nil {
		return ""
	}
	if len(fields) <= 19 {
		return ""
	}
	startTime := fields[19]
	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	boot := strings.TrimSpace(string(bootID))
	if boot == "" || startTime == "" {
		return ""
	}
	return "linux:" + boot + ":" + startTime
}
