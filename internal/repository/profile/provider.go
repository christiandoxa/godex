package profile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const providerSecretMaxBytes = 2 << 20

func (store *Store) ImportProvider(
	ctx context.Context,
	value profileentity.Profile,
	secrets map[string]string,
	activate bool,
) error {
	if !value.Managed {
		return errors.New("imported provider profiles must use managed storage")
	}
	if err := profileentity.Validate(value); err != nil {
		return err
	}
	if err := validateProviderSecrets(secrets); err != nil {
		return err
	}
	return store.withLock(ctx, func() error {
		return store.importProviderLocked(value, secrets, activate)
	})
}

func (store *Store) importProviderLocked(value profileentity.Profile, secrets map[string]string, activate bool) error {
	state, err := store.readState()
	if err != nil {
		return err
	}
	if err := validateNewProfile(state.Profiles, value); err != nil {
		return err
	}
	staged, err := store.stageProviderHome(secrets)
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	if err := os.Rename(staged, value.CodexHome); err != nil {
		return fmt.Errorf("promote imported provider profile: %w", err)
	}
	state.Profiles = append(state.Profiles, value)
	if activate || state.Active == "" {
		state.Active = value.Name
	}
	if err := store.writeState(state); err != nil {
		_ = os.RemoveAll(value.CodexHome)
		return err
	}
	return nil
}

func (store *Store) ReplaceProvider(
	ctx context.Context,
	name, email string,
	provider profileentity.Provider,
	secrets map[string]string,
	activate bool,
) error {
	if err := validateProviderSecrets(secrets); err != nil {
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
		previousProfile := state.Profiles[index]
		backups, err := readSecretBackups(previousProfile.CodexHome, secrets)
		if err != nil {
			return err
		}
		if err := writeProviderSecrets(previousProfile.CodexHome, secrets); err != nil {
			_ = restoreSecretBackups(previousProfile.CodexHome, backups)
			return err
		}
		state.Profiles[index].Email = email
		state.Profiles[index].Provider = provider
		if activate {
			state.Active = name
		}
		if err := store.writeState(state); err != nil {
			_ = restoreSecretBackups(previousProfile.CodexHome, backups)
			return err
		}
		return nil
	})
}

func (store *Store) ReadProviderSecret(codexHome, name string) (string, error) {
	if err := validateSecretName(name); err != nil {
		return "", err
	}
	path := filepath.Join(codexHome, name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", errors.New("provider secret file is unavailable")
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > providerSecretMaxBytes {
		return "", errors.New("provider secret must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("provider secret file is unavailable")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, providerSecretMaxBytes+1))
	if err != nil || len(content) > providerSecretMaxBytes {
		return "", errors.New("provider secret file is unavailable")
	}
	return string(content), nil
}

func (store *Store) stageProviderHome(secrets map[string]string) (string, error) {
	staged, err := os.MkdirTemp(store.profilesRoot(), ".provider-import-")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(staged, 0o700); err != nil {
		_ = os.RemoveAll(staged)
		return "", err
	}
	if err := writeProviderSecrets(staged, secrets); err != nil {
		_ = os.RemoveAll(staged)
		return "", err
	}
	return staged, nil
}

func writeProviderSecrets(home string, secrets map[string]string) error {
	for name, text := range secrets {
		if _, err := fileutil.AtomicWrite(filepath.Join(home, name), []byte(text)); err != nil {
			return err
		}
	}
	return nil
}

type secretBackup struct {
	name    string
	content []byte
	exists  bool
}

func readSecretBackups(home string, secrets map[string]string) ([]secretBackup, error) {
	backups := make([]secretBackup, 0, len(secrets))
	for name := range secrets {
		path := filepath.Join(home, name)
		content, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			backups = append(backups, secretBackup{name: name})
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(content) > providerSecretMaxBytes {
			return nil, errors.New("existing provider secret exceeds safe size limit")
		}
		backups = append(backups, secretBackup{name: name, content: content, exists: true})
	}
	return backups, nil
}

func restoreSecretBackups(home string, backups []secretBackup) error {
	var restoreErr error
	for _, backup := range backups {
		path := filepath.Join(home, backup.name)
		if backup.exists {
			_, err := fileutil.AtomicWrite(path, backup.content)
			restoreErr = errors.Join(restoreErr, err)
		} else if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			restoreErr = errors.Join(restoreErr, err)
		}
	}
	return restoreErr
}

func validateProviderSecrets(secrets map[string]string) error {
	for name, text := range secrets {
		if err := validateSecretName(name); err != nil {
			return err
		}
		if len(text) == 0 || len(text) > providerSecretMaxBytes {
			return fmt.Errorf("provider secret %q is empty or exceeds safe size limit", name)
		}
	}
	return nil
}

func validateSecretName(name string) error {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." || filepath.IsAbs(name) || strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("unsafe provider secret file name %q", name)
	}
	return nil
}
