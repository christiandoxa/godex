package profile

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

func (store *Store) Acquire(ctx context.Context, name string) (func() error, error) {
	if _, err := store.Resolve(ctx, name); err != nil {
		return nil, err
	}
	if err := ensureDirectory(store.leasesRoot()); err != nil {
		return nil, err
	}
	release, err := lockfile.TryRead(store.leasePath(name))
	if err != nil {
		return nil, err
	}
	return release, nil
}

func (store *Store) acquireMutation(name string) (func() error, error) {
	if err := ensureDirectory(store.leasesRoot()); err != nil {
		return nil, err
	}
	release, err := lockfile.TryAcquire(store.leasePath(name))
	if errors.Is(err, lockfile.ErrBusy) {
		return nil, fmt.Errorf("profile %q is in use by another command", name)
	}
	return release, err
}

func (store *Store) leasesRoot() string { return filepath.Join(store.root, "profile-leases") }
func (store *Store) leasePath(name string) string {
	return filepath.Join(store.leasesRoot(), name+".lock")
}
