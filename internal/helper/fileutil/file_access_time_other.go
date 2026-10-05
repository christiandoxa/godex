//go:build !linux && !darwin

package fileutil

import (
	"os"
	"time"
)

func fileAccessTime(info os.FileInfo) time.Time {
	return info.ModTime()
}
