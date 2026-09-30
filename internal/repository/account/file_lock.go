package account

import (
	"context"
	"errors"
	"os"
	"time"
)

var errFileBusy = errors.New("file is locked by another process")

// tryFileLock owns an OS lock until release. Never unlink its persistent file.
func tryFileLock(path string) (func() error, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockDescriptor(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file.Close, nil
}

func acquireFileLock(ctx context.Context, path string) (func() error, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		release, err := tryFileLock(path)
		if !errors.Is(err, errFileBusy) {
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
