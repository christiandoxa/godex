//go:build darwin

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
	return stat.Ctimespec.Sec, stat.Ctimespec.Nsec, stat.Ino
}
