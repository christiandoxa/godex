//go:build linux || darwin

package codex

import (
	"os"
	"syscall"
)

func importCurrentEntryIsSafe(os.FileInfo) bool { return true }

func importCurrentDirectoryTrusted(_ string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	uid := uint32(stat.Uid)
	euid := uint32(os.Geteuid())
	trustedOwner := uid == euid || uid == 0
	stickyRoot := uid == 0 && info.Mode()&os.ModeSticky != 0
	return trustedOwner && (info.Mode().Perm()&0o022 == 0 || stickyRoot)
}

func importCurrentPrivateFileTrusted(_ string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return uint32(stat.Uid) == uint32(os.Geteuid()) && info.Mode().Perm()&0o077 == 0
}
