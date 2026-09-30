package lockfile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

var ErrBusy = errors.New("file is locked by another process")

// TryAcquire owns an OS lock until release. Never unlink its persistent file.
func TryAcquire(path string) (func() error, error) { return tryAcquire(path, false) }

// TryRead lets managed children share a home while mutations require exclusivity.
func TryRead(path string) (func() error, error) { return tryAcquire(path, true) }

func tryAcquire(path string, shared bool) (func() error, error) {
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("lock path must be a regular file")
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.OpenFile(filepath.Base(path), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockDescriptor(file, shared); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file.Close, nil
}

func Acquire(ctx context.Context, path string) (func() error, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		release, err := TryAcquire(path)
		if !errors.Is(err, ErrBusy) {
			return release, err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
