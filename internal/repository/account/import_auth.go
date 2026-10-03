package account

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const importedAuthRollbackDir = ".godex-import-backup-"

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

func (store *FileStore) PrepareImportedAuthRollback(ctx context.Context, accountID, id string) error {
	if !validImportedAuthRollbackID(id) {
		return errors.New("invalid imported authentication rollback ID")
	}
	return store.withLock(ctx, func() error {
		home, release, err := store.lockedAccountHome(accountID, id)
		if err != nil {
			return err
		}
		defer release()
		content, err := readExistingImportedAuth(filepath.Join(home, authFileName))
		if err != nil {
			return err
		}
		defer clearImportedAuth(content)
		return writeImportedAuthRollback(home, id, content)
	})
}

func readExistingImportedAuth(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 2<<20 {
		return nil, errors.New("existing account authentication is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("existing account authentication is not private")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("existing account authentication is unavailable")
	}
	content, readErr := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	err = errors.Join(readErr, file.Close())
	if err != nil || len(content) == 0 || len(content) > 2<<20 {
		clearImportedAuth(content)
		return nil, errors.New("existing account authentication is unavailable")
	}
	return content, nil
}

func writeImportedAuthRollback(home, id string, content []byte) error {
	backupRoot := filepath.Join(home, importedAuthRollbackDir+id)
	if err := os.Mkdir(backupRoot, 0o700); err != nil {
		return fmt.Errorf("create imported authentication rollback directory: %w", err)
	}
	if _, err := fileutil.AtomicWrite(filepath.Join(backupRoot, authFileName), content); err != nil {
		_ = os.RemoveAll(backupRoot)
		return fmt.Errorf("save imported authentication rollback backup: %w", err)
	}
	return fileutil.SyncDirectory(backupRoot)
}

func (store *FileStore) RestoreImportedAuthRollback(ctx context.Context, accountID, id string) error {
	return store.withLock(ctx, func() error {
		home, release, err := store.lockedAccountHome(accountID, id)
		if err != nil {
			return err
		}
		defer release()
		backup, err := readImportedAuthRollback(filepath.Join(home, importedAuthRollbackDir+id, authFileName))
		if err != nil {
			return err
		}
		defer clearImportedAuth(backup)
		authPath := filepath.Join(home, authFileName)
		if err := validateImportedAuthTarget(authPath); err != nil {
			return err
		}
		if _, err := fileutil.AtomicWrite(authPath, backup); err != nil {
			return fmt.Errorf("restore imported account authentication: %w", err)
		}
		return nil
	})
}

func (store *FileStore) CleanupImportedAuthRollback(ctx context.Context, accountID, id string) error {
	return store.withLock(ctx, func() error {
		home, release, err := store.lockedAccountHome(accountID, id)
		if err != nil {
			return err
		}
		defer release()
		backupRoot := filepath.Join(home, importedAuthRollbackDir+id)
		if info, err := os.Lstat(backupRoot); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("imported authentication rollback backup is unavailable")
		}
		return os.RemoveAll(backupRoot)
	})
}

func (store *FileStore) lockedAccountHome(accountID, id string) (string, func() error, error) {
	if !validImportedAuthRollbackID(id) || len(accountID) != 32 {
		return "", nil, errors.New("invalid imported authentication rollback target")
	}
	if _, err := hex.DecodeString(accountID); err != nil {
		return "", nil, errors.New("invalid imported authentication rollback target")
	}
	state, err := store.readState()
	if err != nil {
		return "", nil, err
	}
	index, err := resolveIndex(state.Accounts, accountID)
	if err != nil || state.Accounts[index].ID != accountID {
		return "", nil, errors.New("imported authentication account is unavailable")
	}
	release, err := store.acquireProfile(accountID)
	if err != nil {
		return "", nil, err
	}
	home := store.CodexHome(accountID)
	if err := ownerOnlyDirectory(home); err != nil {
		_ = release()
		return "", nil, err
	}
	return home, release, nil
}

func readImportedAuthRollback(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 2<<20 {
		return nil, errors.New("imported authentication rollback backup is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("imported authentication rollback backup is not private")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("imported authentication rollback backup is unavailable")
	}
	content, readErr := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	err = errors.Join(readErr, file.Close())
	if err != nil || len(content) == 0 || len(content) > 2<<20 {
		clearImportedAuth(content)
		return nil, errors.New("imported authentication rollback backup is unavailable")
	}
	return content, nil
}

func validateImportedAuthTarget(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 2<<20 {
		return errors.New("account authentication target is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return errors.New("account authentication target is not private")
	}
	return nil
}

func validImportedAuthRollbackID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func clearImportedAuth(content []byte) {
	for index := range content {
		content[index] = 0
	}
}
