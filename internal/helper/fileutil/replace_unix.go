//go:build !windows

package fileutil

import "os"

func Replace(source, destination string) error {
	return os.Rename(source, destination)
}

func SyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
