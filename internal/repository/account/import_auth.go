package account

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

func (store *FileStore) ReplaceImportedAuth(ctx context.Context, selector string, authJSON []byte) error {
	if len(authJSON) == 0 || len(authJSON) > 2<<20 {
		return errors.New("imported authentication is invalid or too large")
	}
	return store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		index, err := resolveIndex(state.Accounts, selector)
		if err != nil {
			return err
		}
		account := state.Accounts[index]
		release, err := store.acquireProfile(account.ID)
		if err != nil {
			return err
		}
		defer release()
		home := store.CodexHome(account.ID)
		if err := ownerOnlyDirectory(home); err != nil {
			return err
		}
		if _, err := fileutil.AtomicWrite(filepath.Join(home, "auth.json"), authJSON); err != nil {
			return fmt.Errorf("replace imported account authentication: %w", err)
		}
		return nil
	})
}
