//go:build linux

package codex

import (
	"os"
	"syscall"
	"time"
)

func sessionFileAccessTime(info os.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
	}
	return info.ModTime()
}
