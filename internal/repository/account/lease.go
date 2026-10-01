package account

import (
	"context"
	"errors"
	"fmt"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
	"os"
	"path/filepath"
)

// AcquireProfiles pins homes against removal and credential replacement.
func (store *FileStore) AcquireProfiles(ctx context.Context, ids []string) (func() error, error) {
	var releases []func() error
	err := store.withLock(ctx, func() error {
		var acquireErr error
		releases, acquireErr = store.acquireProfileReads(ids)
		return acquireErr
	})
	if err != nil {
		return nil, errors.Join(err, releaseProfileLocks(releases))
	}
	return func() error { return releaseProfileLocks(releases) }, nil
}

func (store *FileStore) acquireProfileReads(ids []string) ([]func() error, error) {
	state, err := store.readState()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(store.root, "leases")
	if err := ensurePrivateDirectory(directory); err != nil {
		return nil, err
	}
	releases := make([]func() error, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := resolveIndex(state.Accounts, id); err != nil {
			return releases, err
		}
		unlock, err := lockfile.TryRead(filepath.Join(directory, id+".lock"))
		if err != nil {
			return releases, err
		}
		releases = append(releases, unlock)
	}
	return releases, nil
}

func releaseProfileLocks(releases []func() error) error {
	var err error
	for i := len(releases) - 1; i >= 0; i-- {
		err = errors.Join(err, releases[i]())
	}
	return err
}

func (store *FileStore) acquireProfile(id string) (func() error, error) {
	info, err := os.Lstat(store.accountDir(id))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("managed profile must be a real directory")
	}
	directory := filepath.Join(store.root, "leases")
	if err := ensurePrivateDirectory(directory); err != nil {
		return nil, err
	}
	release, err := lockfile.TryAcquire(filepath.Join(directory, id+".lock"))
	if errors.Is(err, lockfile.ErrBusy) {
		return nil, fmt.Errorf("account %q is in use by another command", id)
	}
	return release, err
}

func (store *FileStore) AcquireProfileMutation(ctx context.Context, id string) (func() error, error) {
	var release func() error
	err := store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		if _, err := resolveIndex(state.Accounts, id); err != nil {
			return err
		}
		release, err = store.acquireProfile(id)
		return err
	})
	return release, err
}
