//go:build windows

package codex

import "os"

func sessionFileChangedIdentity(info os.FileInfo) (int64, int64, uint64) {
	seconds, nanos := sessionModifiedParts(info.ModTime())
	return int64(seconds), int64(nanos), 0
}
