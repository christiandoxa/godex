package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	stateVersion              = 1
	profileDoesNotExistFormat = "profile %q does not exist"
)

var ErrNoActiveProfile = errors.New("no active profile")

type Store struct {
	root string
}

type stateFile struct {
	Version  int                     `json:"version"`
	Active   string                  `json:"active_profile,omitempty"`
	Profiles []profileentity.Profile `json:"profiles"`
}

func NewStore(root string) *Store { return &Store{root: filepath.Clean(root)} }

func (store *Store) Prepare() error {
	if !filepath.IsAbs(store.root) || store.root == filepath.Dir(store.root) {
		return errors.New("GODEX_HOME must be an absolute non-root path")
	}
	if err := ensureDirectory(store.root); err != nil {
		return err
	}
	return ensureDirectory(store.profilesRoot())
}

func (store *Store) ManagedHome(name string) string {
	return filepath.Join(store.profilesRoot(), name)
}

func (store *Store) List(ctx context.Context) ([]profileentity.Profile, error) {
	state, err := store.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	values := append([]profileentity.Profile(nil), state.Profiles...)
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	return values, nil
}

func (store *Store) Current(ctx context.Context) (profileentity.Profile, error) {
	state, err := store.snapshot(ctx)
	if err != nil {
		return profileentity.Profile{}, err
	}
	if state.Active == "" {
		return profileentity.Profile{}, ErrNoActiveProfile
	}
	for _, current := range state.Profiles {
		if current.Name == state.Active {
			return current, nil
		}
	}
	return profileentity.Profile{}, errors.New("active profile metadata is inconsistent")
}

func (store *Store) Resolve(ctx context.Context, name string) (profileentity.Profile, error) {
	state, err := store.snapshot(ctx)
	if err != nil {
		return profileentity.Profile{}, err
	}
	for _, current := range state.Profiles {
		if current.Name == name {
			return current, nil
		}
	}
	return profileentity.Profile{}, fmt.Errorf(profileDoesNotExistFormat, name)
}

func (store *Store) Create(ctx context.Context, value profileentity.Profile, source string, insecure, activate bool) error {
	if err := profileentity.Validate(value); err != nil {
		return err
	}
	return store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		if err := validateNewProfile(state.Profiles, value); err != nil {
			return err
		}
		created, err := store.prepareProfileHome(value, source, insecure)
		if err != nil {
			return err
		}
		state.Profiles = append(state.Profiles, value)
		if activate || state.Active == "" {
			state.Active = value.Name
		}
		if err := store.writeState(state); err != nil {
			if created {
				_ = os.RemoveAll(value.CodexHome)
			}
			return err
		}
		return nil
	})
}

func (store *Store) SetActive(ctx context.Context, name string) (profileentity.Profile, error) {
	var selected profileentity.Profile
	err := store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		for _, current := range state.Profiles {
			if current.Name == name {
				selected = current
				state.Active = name
				return store.writeState(state)
			}
		}
		return fmt.Errorf(profileDoesNotExistFormat, name)
	})
	return selected, err
}

func (store *Store) ClearActive(ctx context.Context) error {
	return store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		state.Active = ""
		return store.writeState(state)
	})
}

func (store *Store) snapshot(ctx context.Context) (stateFile, error) {
	if err := ctx.Err(); err != nil {
		return stateFile{}, err
	}
	if err := store.Prepare(); err != nil {
		return stateFile{}, err
	}
	release, err := lockfile.Acquire(ctx, store.lockPath())
	if err != nil {
		return stateFile{}, err
	}
	defer release()
	if _, err := store.recoverImportAuthJournalLocked(); err != nil {
		return stateFile{}, err
	}
	return store.readState()
}

func (store *Store) withLock(ctx context.Context, operation func() error) error {
	return store.withLockRecovering(ctx, func(int) error { return operation() })
}

func (store *Store) withLockRecovering(ctx context.Context, operation func(int) error) error {
	if err := store.Prepare(); err != nil {
		return err
	}
	release, err := lockfile.Acquire(ctx, store.lockPath())
	if err != nil {
		return err
	}
	defer release()
	recovered, err := store.recoverImportAuthJournalLocked()
	if err != nil {
		return err
	}
	return operation(recovered)
}

func (store *Store) profilesRoot() string { return filepath.Join(store.root, "profiles") }
func (store *Store) statePath() string    { return filepath.Join(store.root, "profiles.json") }
func (store *Store) lockPath() string     { return filepath.Join(store.root, "profiles.guard") }

func ensureDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create profile directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("profile path %s must be a real directory", path)
	}
	return os.Chmod(path, 0o700)
}

func validateNewProfile(existing []profileentity.Profile, value profileentity.Profile) error {
	for _, current := range existing {
		if current.Name == value.Name {
			return fmt.Errorf("profile %q already exists", value.Name)
		}
		if filepath.Clean(current.CodexHome) == filepath.Clean(value.CodexHome) {
			return fmt.Errorf("CODEX_HOME %s is already registered", value.CodexHome)
		}
	}
	return nil
}

func (store *Store) HasActive(ctx context.Context) (bool, error) {
	state, err := store.snapshot(ctx)
	if err != nil {
		return false, err
	}
	return state.Active != "", nil
}
