//go:build windows

package runtimebroker

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func processBirthIdentity(pid uint32) string {
	if pid == 0 {
		return ""
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return ""
	}
	ticks := uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)
	return fmt.Sprintf("windows:%d", ticks)
}
