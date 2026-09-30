package account

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

const stateVersion = 1

type FileStore struct {
	root string
	now  func() time.Time
}

type stateFile struct {
	Version         int              `json:"version"`
	ActiveAccountID string           `json:"active_account_id,omitempty"`
	RotationCursor  uint64           `json:"rotation_cursor"`
	Accounts        []entity.Account `json:"accounts"`
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: filepath.Clean(root), now: time.Now}
}

func (store *FileStore) Prepare() error {
	if err := store.checkRoot(); err != nil {
		return err
	}
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", store.root, err)
	}
	for _, directory := range []string{store.root, store.accountsDir(), store.TempDir()} {
		if err := ensurePrivateDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func (store *FileStore) Root() string {
	return store.root
}

func (store *FileStore) TempDir() string {
	return filepath.Join(store.root, "tmp")
}

func (store *FileStore) CodexHome(accountID string) string {
	return filepath.Join(store.accountDir(accountID), "codex")
}

func (store *FileStore) List(ctx context.Context) ([]entity.Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	state, err := store.readSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	accounts := append([]entity.Account(nil), state.Accounts...)
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Name != accounts[j].Name {
			return accounts[i].Name < accounts[j].Name
		}
		return accounts[i].ID < accounts[j].ID
	})
	return accounts, nil
}

func (store *FileStore) Resolve(ctx context.Context, selector string) (entity.Account, error) {
	if err := ctx.Err(); err != nil {
		return entity.Account{}, err
	}
	state, err := store.readSnapshot(ctx)
	if err != nil {
		return entity.Account{}, err
	}
	index, err := resolveIndex(state.Accounts, selector)
	if err != nil {
		return entity.Account{}, err
	}
	return state.Accounts[index], nil
}

func (store *FileStore) SelectForLaunch(ctx context.Context, selector string) (entity.Account, error) {
	var selected entity.Account
	err := store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}

		index, err := selectIndex(state, selector)
		if err != nil {
			return err
		}
		selected = state.Accounts[index]
		if !selected.Enabled {
			return fmt.Errorf("account %q is disabled", selected.Name)
		}

		selected.LastUsedAt = store.now().UTC()
		selected.UpdatedAt = maxTime(selected.UpdatedAt, selected.LastUsedAt)
		state.Accounts[index] = selected
		state.ActiveAccountID = selected.ID
		state.RotationCursor = nextCursor(state.Accounts, index)
		_, err = store.writeState(state)
		return err
	})
	return selected, err
}

func (store *FileStore) SetActive(ctx context.Context, selector string) (entity.Account, error) {
	var selected entity.Account
	err := store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		index, err := resolveIndex(state.Accounts, selector)
		if err != nil {
			return err
		}
		selected = state.Accounts[index]
		if !selected.Enabled {
			return fmt.Errorf("account %q is disabled", selected.Name)
		}
		state.ActiveAccountID = selected.ID
		state.RotationCursor = enabledPosition(state.Accounts, index)
		_, err = store.writeState(state)
		return err
	})
	return selected, err
}

func (store *FileStore) accountsDir() string {
	return filepath.Join(store.root, "accounts")
}

func (store *FileStore) accountDir(accountID string) string {
	return filepath.Join(store.accountsDir(), accountID)
}

func (store *FileStore) statePath() string {
	return filepath.Join(store.root, "state.json")
}

func (store *FileStore) checkRoot() error {
	if !filepath.IsAbs(store.root) {
		return errors.New("GODEX_HOME must be an absolute path")
	}
	if store.root == filepath.Dir(store.root) {
		return errors.New("GODEX_HOME must not be the filesystem root")
	}
	info, err := os.Lstat(store.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Godex home: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("GODEX_HOME must not be a symbolic link")
	}
	if !info.IsDir() {
		return errors.New("GODEX_HOME is not a directory")
	}
	return nil
}

func ownerOnlyDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("godex path %s is not a real directory", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure directory %s: %w", path, err)
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.Mkdir(path, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
	case err != nil:
		return fmt.Errorf("inspect %s: %w", path, err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("godex path %s must not be a symbolic link", path)
	case !info.IsDir():
		return fmt.Errorf("godex path %s is not a directory", path)
	}
	return ownerOnlyDirectory(path)
}
