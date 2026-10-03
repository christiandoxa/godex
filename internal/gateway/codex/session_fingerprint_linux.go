//go:build linux

package codex

import (
	"os"
	"syscall"
)

func sessionFileChangedIdentity(info os.FileInfo) (int64, int64, uint64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		modified := info.ModTime()
		return modified.Unix(), int64(modified.Nanosecond()), 0
	}
	return stat.Ctim.Sec, stat.Ctim.Nsec, stat.Ino
}
