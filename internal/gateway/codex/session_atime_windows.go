//go:build windows

package codex

import (
	"os"
	"syscall"
	"time"
)

func sessionFileAccessTime(info os.FileInfo) time.Time {
	if data, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, data.LastAccessTime.Nanoseconds())
	}
	return info.ModTime()
}
