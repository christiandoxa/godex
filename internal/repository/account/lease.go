package account

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

// AcquireProfiles pins homes against removal and credential replacement.
func (store *FileStore) AcquireProfiles(ctx context.Context, ids []string) (func() error, error) {
	var releases []func() error
	release := func() error {
		var err error
		for i := len(releases) - 1; i >= 0; i-- {
			err = errors.Join(err, releases[i]())
		}
		return err
	}
	err := store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			if _, err := resolveIndex(state.Accounts, id); err != nil {
				return err
			}
			unlock, err := store.acquireProfile(id)
			if err != nil {
				return err
			}
			releases = append(releases, unlock)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Join(err, release())
	}
	return release, nil
}

func (store *FileStore) acquireProfile(id string) (func() error, error) {
	directory := filepath.Join(store.root, "leases")
	if err := ensurePrivateDirectory(directory); err != nil {
		return nil, err
	}
	release, err := tryFileLock(filepath.Join(directory, id+".lock"))
	if errors.Is(err, errFileBusy) {
		return nil, fmt.Errorf("account %q is in use by another command", id)
	}
	return release, err
}
