package fileutil

import (
	"errors"
	"os"
	"path/filepath"
)

// AtomicWrite fsyncs private data and reports whether replacement committed.
func AtomicWrite(path string, content []byte) (bool, error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".atomic-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(content); err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return false, err
	}
	if err := Replace(file.Name(), path); err != nil {
		return false, err
	}
	return true, SyncDirectory(filepath.Dir(path))
}
