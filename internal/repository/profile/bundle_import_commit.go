package profile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func (store *Store) BundleImportActionCommitted(
	ctx context.Context,
	action profilemodel.ImportLifecycleAction,
	id string,
) (bool, error) {
	if action.AccountID != "" || !validImportID(id) || profileentity.ValidateName(action.Name) != nil {
		return false, errors.New("invalid profile import lifecycle commit check")
	}
	var committed bool
	err := store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		index := profileIndex(state.Profiles, action.Name)
		if index < 0 {
			return nil
		}
		expected := importLifecycleProfile(action.After, action.Name)
		if state.Profiles[index] != expected {
			return nil
		}
		matches, err := bundleImportFilesMatch(expected.CodexHome, action.Files)
		if err != nil || !matches {
			return err
		}
		if action.Create {
			owned, err := readBundleImportOwner(expected.CodexHome, id)
			if err != nil {
				return err
			}
			if !owned {
				return nil
			}
		}
		committed = true
		return nil
	})
	return committed, err
}

func bundleImportFilesMatch(home string, files []profilemodel.ImportLifecycleFile) (bool, error) {
	info, err := os.Lstat(home)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, errors.New("profile import lifecycle home is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return false, errors.New("profile import lifecycle home is not private")
	}
	for _, file := range files {
		matches, err := bundleImportFileMatches(home, file)
		if err != nil || !matches {
			return matches, err
		}
	}
	return true, nil
}

func bundleImportFileMatches(home string, file profilemodel.ImportLifecycleFile) (bool, error) {
	if err := validateImportRollbackFile(file.Path); err != nil || len(file.SHA256) != sha256.Size*2 {
		return false, errors.New("invalid profile import lifecycle file entry")
	}
	if _, err := hex.DecodeString(file.SHA256); err != nil {
		return false, errors.New("invalid profile import lifecycle file digest")
	}
	path := filepath.Join(home, file.Path)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > providerSecretMaxBytes {
		return false, errors.New("profile import lifecycle file is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return false, errors.New("profile import lifecycle file is not private")
	}
	f, err := os.Open(path)
	if err != nil {
		return false, errors.New("profile import lifecycle file is unavailable")
	}
	content, readErr := io.ReadAll(io.LimitReader(f, providerSecretMaxBytes+1))
	err = errors.Join(readErr, f.Close())
	if err != nil || len(content) > providerSecretMaxBytes {
		clearBytes(content)
		return false, errors.New("profile import lifecycle file is unavailable")
	}
	digest := sha256.Sum256(content)
	clearBytes(content)
	return hex.EncodeToString(digest[:]) == file.SHA256, nil
}
