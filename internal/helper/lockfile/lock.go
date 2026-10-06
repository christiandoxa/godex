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

// TryAcquireExisting locks a pre-created regular file and never creates it.
func TryAcquireExisting(path string) (func() error, error) {
	file, err := openExistingLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := lockDescriptor(file, false); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file.Close, nil
}

func tryAcquire(path string, shared bool) (func() error, error) {
	file, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := lockDescriptor(file, shared); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file.Close, nil
}

func openExistingLockFile(path string) (*os.File, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := filepath.Base(path)
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("lock path must be a regular file")
	}
	return root.OpenFile(name, os.O_RDWR, 0o600)
}

func openLockFile(path string) (*os.File, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := filepath.Base(path)
	for attempt := 0; attempt < 4; attempt++ {
		file, retry, err := openLockFileAttempt(root, name)
		if !retry {
			return file, err
		}
	}
	return nil, errors.New("lock file changed while opening")
}

func openLockFileAttempt(root *os.Root, name string) (*os.File, bool, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		file, createErr := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if errors.Is(createErr, os.ErrExist) || errors.Is(createErr, os.ErrNotExist) {
			return nil, true, nil
		}
		return file, false, createErr
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, errors.New("lock path must be a regular file")
	}
	file, openErr := root.OpenFile(name, os.O_RDWR, 0o600)
	if errors.Is(openErr, os.ErrNotExist) {
		return nil, true, nil
	}
	return file, false, openErr
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
