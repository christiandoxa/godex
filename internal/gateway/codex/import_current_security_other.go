//go:build !linux && !darwin && !windows

package codex

import "os"

func importCurrentEntryIsSafe(os.FileInfo) bool                { return true }
func importCurrentDirectoryTrusted(string, os.FileInfo) bool   { return true }
func importCurrentPrivateFileTrusted(string, os.FileInfo) bool { return true }
