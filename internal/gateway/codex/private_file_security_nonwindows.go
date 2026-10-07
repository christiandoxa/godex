//go:build !windows

package codex

import "os"

func securePrivateFile(file *os.File, _ string) error {
	return file.Chmod(0o600)
}
