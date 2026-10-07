//go:build !linux && !darwin && !windows

package codex

import "os"

func importCurrentEntryIsSafe(os.FileInfo) bool                { return true }
func importCurrentSymlinkTrusted(os.FileInfo) bool             { return false }
func importCurrentDirectoryTrusted(string, os.FileInfo) bool   { return true }
func importCurrentPrivateFileTrusted(string, os.FileInfo) bool { return true }
