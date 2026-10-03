//go:build linux

package codex

import (
	"os"
	"syscall"
)

func sessionRepairPlatformRevision(info os.FileInfo) [3]int64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return [3]int64{}
	}
	return [3]int64{stat.Ctim.Sec, stat.Ctim.Nsec, int64(stat.Mode)}
}
