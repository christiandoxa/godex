//go:build !windows

package codex

import "os"

func syncSessionAttachmentDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
