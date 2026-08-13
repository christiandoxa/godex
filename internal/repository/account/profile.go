package account

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func (store *FileStore) replaceProfile(accountID, stagedCodexHome string) (string, func() error, error) {
	accountDirectory := store.accountDir(accountID)
	backup, err := store.transactionPath(accountDirectory, "backup")
	if err != nil {
		return "", func() error { return nil }, err
	}
	if err := validateStagedProfile(stagedCodexHome); err != nil {
		return "", func() error { return nil }, err
	}
	hadExisting, err := stageExistingProfile(accountDirectory, backup)
	if err != nil {
		return "", func() error { return nil }, err
	}
	rollback := func() error {
		return store.rollbackProfile(accountDirectory, backup, hadExisting)
	}
	if err := store.promoteProfile(accountDirectory, store.CodexHome(accountID), stagedCodexHome, rollback); err != nil {
		return "", func() error { return nil }, err
	}
	if !hadExisting {
		backup = ""
	}
	return backup, rollback, nil
}

func validateStagedProfile(path string) error {
	stagedInfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect staged Codex home: %w", err)
	}
	if stagedInfo.Mode()&os.ModeSymlink != 0 || !stagedInfo.IsDir() {
		return errors.New("staged Codex home must be a real directory")
	}
	return nil
}

func stageExistingProfile(accountDirectory, backup string) (bool, error) {
	if _, err := os.Lstat(accountDirectory); err == nil {
		if err := os.Rename(accountDirectory, backup); err != nil {
			return false, fmt.Errorf("stage existing account profile: %w", err)
		}
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect existing account profile: %w", err)
	}
	return false, nil
}

func (store *FileStore) promoteProfile(accountDirectory, codexHome, stagedCodexHome string, rollback func() error) error {
	if err := os.Mkdir(accountDirectory, 0o700); err != nil {
		return fmt.Errorf("create account directory: %w", rollbackProfileError(err, rollback))
	}
	if err := os.Rename(stagedCodexHome, codexHome); err != nil {
		return fmt.Errorf("promote Codex profile: %w", rollbackProfileError(err, rollback))
	}
	if err := secureCodexHome(codexHome); err != nil {
		return rollbackProfileError(err, rollback)
	}
	if err := syncDirectory(store.accountsDir()); err != nil {
		return fmt.Errorf("sync promoted account profile: %w", rollbackProfileError(err, rollback))
	}
	return nil
}

func rollbackProfileError(err error, rollback func() error) error {
	if rollbackErr := rollback(); rollbackErr != nil {
		return errors.Join(err, rollbackErr)
	}
	return err
}

func (store *FileStore) rollbackProfile(accountDirectory, backup string, hadExisting bool) error {
	var rollbackErr error
	if err := os.RemoveAll(accountDirectory); err != nil && !errors.Is(err, os.ErrNotExist) {
		rollbackErr = fmt.Errorf("remove promoted account profile: %w", err)
	}
	if hadExisting {
		if err := os.Rename(backup, accountDirectory); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore previous account profile: %w", err))
		}
	}
	if err := syncDirectory(store.accountsDir()); err != nil {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("sync account profile rollback: %w", err))
	}
	return rollbackErr
}

func (store *FileStore) stageRemoval(accountID string) (string, func() error, error) {
	accountDirectory := store.accountDir(accountID)
	trash, err := store.transactionPath(accountDirectory, "remove")
	if err != nil {
		return "", func() error { return nil }, err
	}
	if err := os.Rename(accountDirectory, trash); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", func() error { return nil }, nil
		}
		return "", func() error { return nil }, fmt.Errorf("stage account removal: %w", err)
	}
	if err := syncDirectory(store.accountsDir()); err != nil {
		restoreErr := os.Rename(trash, accountDirectory)
		if restoreErr == nil {
			restoreErr = syncDirectory(store.accountsDir())
		}
		if restoreErr != nil {
			return "", func() error { return nil }, errors.Join(
				fmt.Errorf("sync staged account removal: %w", err),
				fmt.Errorf("restore staged account removal: %w", restoreErr),
			)
		}
		return "", func() error { return nil }, fmt.Errorf("sync staged account removal: %w", err)
	}
	return trash, func() error {
		if err := os.Rename(trash, accountDirectory); err != nil {
			return err
		}
		return syncDirectory(store.accountsDir())
	}, nil
}

func secureCodexHome(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect Codex home: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Codex home must be a real directory")
	}
	return filepath.WalkDir(path, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || entryInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Codex home contains symbolic link %s", filepath.Base(currentPath))
		}
		mode := os.FileMode(0o600)
		if entryInfo.IsDir() {
			mode = 0o700
		} else if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("Codex home contains unsupported file %s", filepath.Base(currentPath))
		}
		if err := os.Chmod(currentPath, mode); err != nil {
			return fmt.Errorf("secure Codex profile entry %s: %w", filepath.Base(currentPath), err)
		}
		return nil
	})
}

func (store *FileStore) transactionPath(base, suffix string) (string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		candidate := fmt.Sprintf("%s.%s-%d-%d", base, suffix, store.now().UnixNano(), attempt)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect transaction path: %w", err)
		}
	}
	return "", errors.New("unable to allocate profile transaction path")
}
