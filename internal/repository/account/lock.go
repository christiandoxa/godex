package account

import (
	"context"
	"errors"
	"fmt"

	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	lockPollInterval = 25 * time.Millisecond
	staleLockAge     = 2 * time.Minute
)

func (store *FileStore) withLock(ctx context.Context, operation func() error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("acquire Godex state lock: %w", err)
	}
	if err := store.Prepare(); err != nil {
		return err
	}
	unlock, err := acquireFileLock(ctx, filepath.Join(store.root, "state.guard"))
	if err != nil {
		return err
	}
	defer unlock()
	lockPath := filepath.Join(store.root, "state.lock")
	token, err := store.acquireLock(ctx, lockPath)
	if err != nil {
		return err
	}
	defer releaseLock(lockPath, token)
	if err := store.recoverTransaction(); err != nil {
		return err
	}
	return operation()
}

func (store *FileStore) acquireLock(ctx context.Context, lockPath string) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("acquire Godex state lock: %w", err)
		}
		token, acquired, err := tryAcquireLock(lockPath)
		if err != nil {
			return "", err
		}
		if acquired {
			return token, nil
		}
		if stale, checkErr := store.lockIsStale(lockPath); checkErr != nil {
			return "", checkErr
		} else if stale {
			if err := quarantineStaleLock(store.root, lockPath); err != nil {
				return "", fmt.Errorf("remove stale Godex state lock: %w", err)
			}
			continue
		}

		if err := waitForLock(ctx); err != nil {
			return "", err
		}
	}
}

func tryAcquireLock(lockPath string) (string, bool, error) {
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("acquire Godex state lock: %w", err)
	}
	token, err := createLockOwner(lockPath)
	if err != nil {
		_ = os.RemoveAll(lockPath)
		return "", false, err
	}
	return token, true, nil
}

func waitForLock(ctx context.Context) error {
	timer := time.NewTimer(lockPollInterval)
	select {
	case <-ctx.Done():
		timer.Stop()
		return fmt.Errorf("wait for Godex state lock: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func (store *FileStore) lockIsStale(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect Godex state lock: %w", err)
	}
	owner, readErr := os.ReadFile(filepath.Join(path, "owner"))
	if readErr == nil {
		value, _, _ := strings.Cut(string(owner), "-")
		pid, err := strconv.Atoi(value)
		if err != nil || pid <= 0 {
			return false, errors.New("invalid Godex state lock owner")
		}
		return !processAlive(pid), nil
	}
	if !errors.Is(readErr, os.ErrNotExist) {
		return false, readErr
	}
	return store.now().Sub(info.ModTime()) > staleLockAge, nil
}

func createLockOwner(lockPath string) (string, error) {
	token := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	ownerPath := filepath.Join(lockPath, "owner")
	owner, err := os.OpenFile(ownerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create Godex state lock owner: %w", err)
	}
	if _, err := owner.WriteString(token); err != nil {
		_ = owner.Close()
		return "", fmt.Errorf("write Godex state lock owner: %w", err)
	}
	if err := owner.Close(); err != nil {
		return "", fmt.Errorf("close Godex state lock owner: %w", err)
	}
	return token, nil
}

func releaseLock(lockPath, token string) {
	ownerPath := filepath.Join(lockPath, "owner")
	owner, err := os.ReadFile(ownerPath)
	if err != nil || string(owner) != token {
		return
	}
	_ = os.RemoveAll(lockPath)
}

func quarantineStaleLock(root, lockPath string) error {
	marker, err := os.CreateTemp(root, ".stale-lock-*")
	if err != nil {
		return err
	}
	markerPath := marker.Name()
	if err := marker.Close(); err != nil {
		_ = os.Remove(markerPath)
		return err
	}
	if err := os.Remove(markerPath); err != nil {
		return err
	}
	if err := os.Rename(lockPath, markerPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.RemoveAll(markerPath)
}
