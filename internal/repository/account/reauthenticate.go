package account

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Reauthentication replaces credentials only; Codex history and settings remain.
func (store *FileStore) replaceAuthentication(accountID, stagedHome string) (string, func() error, error) {
	home := store.CodexHome(accountID)
	if err := ownerOnlyDirectory(home); err != nil {
		return "", nil, err
	}
	if err := secureCodexHome(stagedHome); err != nil {
		return "", nil, err
	}
	auth := filepath.Join(home, "auth.json")
	backup, err := store.transactionPath(auth, "backup")
	if err != nil {
		return "", nil, err
	}
	hadAuth, err := backupAuthentication(auth, backup)
	if err != nil {
		return "", nil, err
	}
	rollback := func() error {
		if hadAuth {
			return replaceFile(backup, auth)
		}
		err := os.Remove(auth)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := replaceFile(filepath.Join(stagedHome, "auth.json"), auth); err != nil {
		_ = os.Remove(backup)
		return "", nil, fmt.Errorf("replace account authentication: %w", err)
	}
	if err := syncDirectory(home); err != nil {
		return "", nil, rollbackProfileError(err, rollback)
	}
	if !hadAuth {
		backup = ""
	}
	return backup, rollback, nil
}

func backupAuthentication(auth, backup string) (bool, error) {
	info, err := os.Lstat(auth)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return false, errors.New("account authentication must be a bounded regular file")
	}
	source, err := os.Open(auth)
	if err != nil {
		return false, err
	}
	defer source.Close()
	target, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false, err
	}
	_, copyErr := io.Copy(target, io.LimitReader(source, (1<<20)+1))
	syncErr := target.Sync()
	closeErr := target.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		_ = os.Remove(backup)
		return false, err
	}
	return true, nil
}
