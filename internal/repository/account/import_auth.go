package account

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const (
	importedAuthRollbackDir      = ".godex-import-backup-"
	importedAuthRollbackMetaFile = "metadata.json"
)

type importedAuthRollbackMetadata struct {
	Version int
	HadAuth bool
	Account accountentity.Account
}

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
		state, err := store.readState()
		if err != nil {
			return err
		}
		index, err := resolveIndex(state.Accounts, accountID)
		if err != nil || state.Accounts[index].ID != accountID {
			return errors.New("imported authentication account is unavailable")
		}
		home, release, err := store.lockedAccountHome(accountID, id)
		if err != nil {
			return err
		}
		defer release()
		content, hadAuth, err := readExistingImportedAuth(filepath.Join(home, authFileName))
		if err != nil {
			return err
		}
		defer clearImportedAuth(content)
		return writeImportedAuthRollback(home, id, content, importedAuthRollbackMetadata{
			Version: 1, HadAuth: hadAuth, Account: state.Accounts[index],
		})
	})
}

func readExistingImportedAuth(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 2<<20 {
		return nil, false, errors.New("existing account authentication is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, false, errors.New("existing account authentication is not private")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, errors.New("existing account authentication is unavailable")
	}
	content, readErr := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	err = errors.Join(readErr, file.Close())
	if err != nil || len(content) == 0 || len(content) > 2<<20 {
		clearImportedAuth(content)
		return nil, false, errors.New("existing account authentication is unavailable")
	}
	return content, true, nil
}

func writeImportedAuthRollback(home, id string, content []byte, metadata importedAuthRollbackMetadata) error {
	backupRoot := filepath.Join(home, importedAuthRollbackDir+id)
	if err := os.Mkdir(backupRoot, 0o700); err != nil {
		return fmt.Errorf("create imported authentication rollback directory: %w", err)
	}
	cleanup := func(err error) error {
		_ = os.RemoveAll(backupRoot)
		return err
	}
	if metadata.HadAuth {
		if _, err := fileutil.AtomicWrite(filepath.Join(backupRoot, authFileName), content); err != nil {
			return cleanup(fmt.Errorf("save imported authentication rollback backup: %w", err))
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return cleanup(errors.New("encode imported authentication rollback metadata"))
	}
	if _, err := fileutil.AtomicWrite(filepath.Join(backupRoot, importedAuthRollbackMetaFile), encoded); err != nil {
		return cleanup(fmt.Errorf("save imported authentication rollback metadata: %w", err))
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
		backupRoot := filepath.Join(home, importedAuthRollbackDir+id)
		metadata, err := readImportedAuthRollbackMetadata(filepath.Join(backupRoot, importedAuthRollbackMetaFile))
		if err != nil {
			return err
		}
		if metadata.Account.ID != accountID {
			return errors.New("imported authentication rollback account changed")
		}
		authPath := filepath.Join(home, authFileName)
		if err := validateImportedAuthTarget(authPath); err != nil {
			return err
		}
		if metadata.HadAuth {
			backup, err := readImportedAuthRollback(filepath.Join(backupRoot, authFileName))
			if err != nil {
				return err
			}
			defer clearImportedAuth(backup)
			if _, err := fileutil.AtomicWrite(authPath, backup); err != nil {
				return fmt.Errorf("restore imported account authentication: %w", err)
			}
		} else if err := os.Remove(authPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove imported account authentication: %w", err)
		}
		state, err := store.readState()
		if err != nil {
			return err
		}
		index, err := resolveIndex(state.Accounts, accountID)
		if err != nil || state.Accounts[index].ID != accountID {
			return errors.New("imported authentication account is unavailable")
		}
		state.Accounts[index] = metadata.Account
		_, err = store.writeState(state)
		return err
	})
}

func readImportedAuthRollbackMetadata(path string) (importedAuthRollbackMetadata, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 64<<10 {
		return importedAuthRollbackMetadata{}, errors.New("imported authentication rollback metadata is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return importedAuthRollbackMetadata{}, errors.New("imported authentication rollback metadata is not private")
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > 64<<10 {
		return importedAuthRollbackMetadata{}, errors.New("imported authentication rollback metadata is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var metadata importedAuthRollbackMetadata
	if err := decoder.Decode(&metadata); err != nil || requireJSONEOF(decoder) != nil ||
		metadata.Version != 1 || accountentity.ValidateAccount(metadata.Account) != nil {
		return importedAuthRollbackMetadata{}, errors.New("invalid imported authentication rollback metadata")
	}
	return metadata, nil
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
