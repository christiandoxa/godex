package profile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const (
	maxProfileAuthBytes    = 2 << 20
	profileAuthUnavailable = "profile authentication is unavailable"
	profileAuthFileName    = "auth.json"
)

func (store *Store) ReadAuthJSON(codexHome string) ([]byte, error) {
	path := filepath.Join(codexHome, profileAuthFileName)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New(profileAuthUnavailable)
	}
	if !info.Mode().IsRegular() || info.Size() > maxProfileAuthBytes {
		return nil, errors.New("profile authentication must be a bounded regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("profile authentication must be private")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New(profileAuthUnavailable)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxProfileAuthBytes+1))
	if err != nil || len(content) > maxProfileAuthBytes {
		clearBytes(content)
		return nil, errors.New(profileAuthUnavailable)
	}
	return content, nil
}

func (store *Store) ImportOpenAI(ctx context.Context, value profileentity.Profile, authJSON []byte, activate bool) error {
	if !value.Managed {
		return errors.New("imported bundle profiles must use managed storage")
	}
	if err := profileentity.Validate(value); err != nil {
		return err
	}
	if err := validateAuthJSONBytes(authJSON); err != nil {
		return err
	}
	return store.withLock(ctx, func() error {
		return store.importOpenAILocked(value, authJSON, activate)
	})
}

func (store *Store) importOpenAILocked(value profileentity.Profile, authJSON []byte, activate bool) error {
	state, err := store.readState()
	if err != nil {
		return err
	}
	if err := validateNewProfile(state.Profiles, value); err != nil {
		return err
	}
	staged, err := store.stageImportedAuthHome(authJSON)
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	if err := os.Rename(staged, value.CodexHome); err != nil {
		return fmt.Errorf("promote imported profile: %w", err)
	}
	state.Profiles = append(state.Profiles, value)
	if activate {
		state.Active = value.Name
	}
	if err := store.writeState(state); err != nil {
		_ = os.RemoveAll(value.CodexHome)
		return err
	}
	return nil
}

func (store *Store) stageImportedAuthHome(authJSON []byte) (string, error) {
	staged, err := os.MkdirTemp(store.profilesRoot(), ".import-")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(staged, 0o700); err != nil {
		_ = os.RemoveAll(staged)
		return "", err
	}
	if _, err := fileutil.AtomicWrite(filepath.Join(staged, profileAuthFileName), authJSON); err != nil {
		_ = os.RemoveAll(staged)
		return "", err
	}
	return staged, nil
}

func (store *Store) ReplaceAuth(ctx context.Context, name string, authJSON []byte) error {
	if err := validateAuthJSONBytes(authJSON); err != nil {
		return err
	}
	return store.withLock(ctx, func() error {
		release, err := store.acquireMutation(name)
		if err != nil {
			return err
		}
		defer release()
		state, err := store.readState()
		if err != nil {
			return err
		}
		index := profileIndex(state.Profiles, name)
		if index < 0 {
			return fmt.Errorf(profileDoesNotExistFormat, name)
		}
		_, err = fileutil.AtomicWrite(filepath.Join(state.Profiles[index].CodexHome, profileAuthFileName), authJSON)
		return err
	})
}

func validateAuthJSONBytes(content []byte) error {
	if len(content) == 0 || len(content) > maxProfileAuthBytes {
		return errors.New("profile authentication is invalid or too large")
	}
	return nil
}
