//go:build windows

package codex

import (
	"os"
	"syscall"
)

func sessionRepairPlatformRevision(info os.FileInfo) [3]int64 {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return [3]int64{}
	}
	return [3]int64{data.CreationTime.Nanoseconds(), data.LastWriteTime.Nanoseconds(), int64(data.FileAttributes)}
}
