package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

func (store *FileStore) ImportedAuthMatches(ctx context.Context, accountID, digestText string) (bool, error) {
	if err := validateImportedAuthCommitTarget(accountID, digestText); err != nil {
		return false, err
	}
	var matches bool
	err := store.withLock(ctx, func() error {
		found, err := store.importedAuthAccountExists(accountID)
		if err != nil || !found {
			return err
		}
		release, err := store.acquireProfile(accountID)
		if err != nil {
			return err
		}
		defer release()
		digest, err := store.importedAuthDigest(accountID)
		if err != nil {
			return err
		}
		matches = digest == digestText
		return nil
	})
	return matches, err
}

func validateImportedAuthCommitTarget(accountID, digestText string) error {
	if len(accountID) != 32 || len(digestText) != sha256.Size*2 {
		return errors.New("invalid imported authentication commit check")
	}
	if _, err := hex.DecodeString(accountID); err != nil {
		return errors.New("invalid imported authentication commit check")
	}
	if _, err := hex.DecodeString(digestText); err != nil {
		return errors.New("invalid imported authentication digest")
	}
	return nil
}

func (store *FileStore) importedAuthAccountExists(accountID string) (bool, error) {
	state, err := store.readState()
	if err != nil {
		return false, err
	}
	for _, account := range state.Accounts {
		if account.ID == accountID {
			return true, nil
		}
	}
	return false, nil
}

func (store *FileStore) importedAuthDigest(accountID string) (string, error) {
	home := store.CodexHome(accountID)
	if err := ownerOnlyDirectory(home); err != nil {
		return "", err
	}
	content, err := readImportedAuthCommitFile(filepath.Join(home, authFileName))
	if err != nil {
		return "", err
	}
	if len(content) == 0 {
		return "", nil
	}
	digest := sha256.Sum256(content)
	clearImportedAuth(content)
	return hex.EncodeToString(digest[:]), nil
}

func readImportedAuthCommitFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 2<<20 {
		return nil, errors.New("imported authentication target is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("imported authentication target is not private")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("imported authentication target is unavailable")
	}
	content, readErr := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	err = errors.Join(readErr, file.Close())
	if err != nil || len(content) == 0 || len(content) > 2<<20 {
		clearImportedAuth(content)
		return nil, errors.New("imported authentication target is unavailable")
	}
	return content, nil
}
