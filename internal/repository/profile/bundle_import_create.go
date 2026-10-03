package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const bundleImportOwnerPrefix = ".godex-import-owner-"

func (store *Store) ImportBundleProfile(
	ctx context.Context,
	value profileentity.Profile,
	files map[string][]byte,
	id string,
) error {
	if !validImportID(id) {
		return errors.New("invalid profile import lifecycle ID")
	}
	if !value.Managed {
		return errors.New("imported bundle profiles must use managed storage")
	}
	if err := profileentity.Validate(value); err != nil {
		return err
	}
	if err := validateBundleImportFiles(value.Provider.Kind, files); err != nil {
		return err
	}
	if _, exists := files[bundleImportOwnerName(id)]; exists {
		return errors.New("profile import file conflicts with lifecycle metadata")
	}
	return store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		if err := validateNewProfile(state.Profiles, value); err != nil {
			return err
		}
		if _, err := os.Lstat(value.CodexHome); err == nil {
			return errors.New("managed profile home already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect managed profile home: %w", err)
		}
		staged, err := store.stageBundleImportHome(files, id)
		if err != nil {
			return err
		}
		defer os.RemoveAll(staged)
		if err := os.Rename(staged, value.CodexHome); err != nil {
			return fmt.Errorf("promote imported profile: %w", err)
		}
		if err := fileutil.SyncDirectory(store.profilesRoot()); err != nil {
			return fmt.Errorf("sync imported profile home: %w", err)
		}
		state.Profiles = append(state.Profiles, value)
		if err := store.writeState(state); err != nil {
			return err
		}
		return nil
	})
}

func (store *Store) CleanupBundleImportOwnerMarker(ctx context.Context, name, id string) error {
	if err := profileentity.ValidateName(name); err != nil || !validImportID(id) {
		return errors.New("invalid profile import lifecycle target")
	}
	return store.withLock(ctx, func() error {
		home := store.ManagedHome(name)
		info, err := os.Lstat(home)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("profile import lifecycle marker home is unavailable")
		}
		owned, err := readBundleImportOwner(home, id)
		if err != nil || !owned {
			return err
		}
		release, err := store.acquireMutation(name)
		if err != nil {
			return err
		}
		defer release()
		owned, err = readBundleImportOwner(home, id)
		if err != nil || !owned {
			return err
		}
		marker := filepath.Join(home, bundleImportOwnerName(id))
		if err := os.Remove(marker); err != nil {
			return fmt.Errorf("remove profile import lifecycle marker: %w", err)
		}
		return fileutil.SyncDirectory(home)
	})
}

func (store *Store) stageBundleImportHome(files map[string][]byte, id string) (string, error) {
	staged, err := os.MkdirTemp(store.profilesRoot(), ".provider-import-")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(staged, 0o700); err != nil {
		_ = os.RemoveAll(staged)
		return "", err
	}
	for name, content := range files {
		if _, err := fileutil.AtomicWrite(filepath.Join(staged, name), content); err != nil {
			_ = os.RemoveAll(staged)
			return "", fmt.Errorf("write imported profile file: %w", err)
		}
	}
	if _, err := fileutil.AtomicWrite(filepath.Join(staged, bundleImportOwnerName(id)), []byte(id)); err != nil {
		_ = os.RemoveAll(staged)
		return "", fmt.Errorf("write profile import lifecycle marker: %w", err)
	}
	return staged, nil
}

func validateBundleImportFiles(kind profileentity.ProviderKind, files map[string][]byte) error {
	if kind == profileentity.ProviderOpenAI {
		if len(files) != 1 || len(files[profileAuthFileName]) == 0 {
			return errors.New("OpenAI bundle import requires auth.json")
		}
		return validateAuthJSONBytes(files[profileAuthFileName])
	}
	for name, content := range files {
		if err := validateSecretName(name); err != nil {
			return err
		}
		if len(content) == 0 || len(content) > providerSecretMaxBytes {
			return fmt.Errorf("provider secret %q is empty or exceeds safe size limit", name)
		}
	}
	return nil
}

func bundleImportOwnerName(id string) string { return bundleImportOwnerPrefix + id }

func readBundleImportOwner(home, id string) (bool, error) {
	path := filepath.Join(home, bundleImportOwnerName(id))
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != int64(len(id)) {
		return false, errors.New("profile import lifecycle marker is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return false, errors.New("profile import lifecycle marker is not private")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != id {
		return false, errors.New("profile import lifecycle marker does not match")
	}
	return true, nil
}

func isBundleImportOwnerFile(name, id string) bool {
	return id != "" && name == bundleImportOwnerName(id)
}
