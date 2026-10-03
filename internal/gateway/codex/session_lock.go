package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	codexSessionMaintenanceLockFile = ".prodex-maintenance.lock"
	codexSessionChildLockTimeout    = 3 * time.Second
)

// SessionLocker coordinates Codex child processes with shared-session maintenance.
type SessionLocker struct{}

func (SessionLocker) TryLockCodexSessionsForMaintenance(codexHome string) (func() error, bool, error) {
	lockPath, err := prepareCodexSessionLockPath(codexHome)
	if err != nil {
		return nil, false, err
	}
	release, err := lockfile.TryAcquire(lockPath)
	if errors.Is(err, lockfile.ErrBusy) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("lock Codex sessions for maintenance: %w", err)
	}
	return release, true, nil
}

func prepareCodexSessionLockPath(codexHome string) (string, error) {
	if err := validateCodexHomePath(codexHome); err != nil {
		return "", err
	}
	sessionsDir := filepath.Join(codexHome, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return "", fmt.Errorf("create Codex sessions directory: %w", err)
	}
	return filepath.Join(sessionsDir, codexSessionMaintenanceLockFile), nil
}

func (SessionLocker) LockCodexSessionsForChild(ctx context.Context, codexHome string) (func() error, error) {
	lockPath, err := prepareCodexSessionLockPath(codexHome)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(codexSessionChildLockTimeout)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		release, err := lockfile.TryRead(lockPath)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, lockfile.ErrBusy) {
			return nil, fmt.Errorf("lock Codex sessions for child: %w", err)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("timed out after %d ms waiting for Codex session lock", codexSessionChildLockTimeout.Milliseconds())
		}
		timer := time.NewTimer(min(10*time.Millisecond, remaining))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
