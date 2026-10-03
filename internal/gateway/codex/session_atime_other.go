//go:build !linux && !darwin && !windows

package codex

import (
	"os"
	"time"
)

func sessionFileAccessTime(info os.FileInfo) time.Time {
	return info.ModTime()
}
