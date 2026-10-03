package profile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const (
	bundleImportJournalDir = ".profile-import-journals"
	bundleImportBackupDir  = ".godex-import-backup-"
	bundleImportRemovalDir = ".godex-import-remove-"
	bundleImportJournalMax = 4 << 20
	bundleImportVersion    = 1
)

func (store *Store) AcquireBundleImportLock(ctx context.Context) (func() error, error) {
	if err := store.Prepare(); err != nil {
		return nil, err
	}
	return lockfile.Acquire(ctx, filepath.Join(store.root, "profile-import-lifecycle.guard"))
}

func (store *Store) WriteBundleImportJournal(journal profilemodel.ImportLifecycleJournal) error {
	if err := validateBundleImportJournal(store, journal); err != nil {
		return err
	}
	root := store.bundleImportJournalRoot()
	if err := ensureDirectory(root); err != nil {
		return err
	}
	content, err := json.Marshal(journal)
	if err != nil {
		return errors.New("encode profile import lifecycle journal")
	}
	if len(content) > bundleImportJournalMax {
		return errors.New("profile import lifecycle journal exceeds safe size limit")
	}
	_, err = fileutil.AtomicWrite(store.bundleImportJournalPath(journal.ID), content)
	if err != nil {
		return fmt.Errorf("write profile import lifecycle journal: %w", err)
	}
	return fileutil.SyncDirectory(root)
}

func (store *Store) BundleImportJournals() ([]profilemodel.ImportLifecycleJournal, error) {
	root := store.bundleImportJournalRoot()
	rootInfo, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, errors.New("profile import lifecycle journal directory is unavailable")
	}
	if runtime.GOOS != "windows" && rootInfo.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("profile import lifecycle journal directory is not private")
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read profile import lifecycle journals: %w", err)
	}
	journals := make([]profilemodel.ImportLifecycleJournal, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !validImportID(id) {
			return nil, errors.New("invalid profile import lifecycle journal filename")
		}
		journal, err := readBundleImportJournal(store, store.bundleImportJournalPath(id))
		if err != nil {
			return nil, err
		}
		if journal.ID != id {
			return nil, errors.New("profile import lifecycle journal ID does not match its filename")
		}
		journals = append(journals, journal)
	}
	return journals, nil
}

func (store *Store) RemoveBundleImportJournal(id string) error {
	if !validImportID(id) {
		return errors.New("invalid profile import lifecycle journal ID")
	}
	path := store.bundleImportJournalPath(id)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove profile import lifecycle journal: %w", err)
	}
	return fileutil.SyncDirectory(store.bundleImportJournalRoot())
}

func (store *Store) PrepareBundleImportRollback(
	ctx context.Context,
	name, id string,
	paths []string,
) error {
	if !validImportID(id) {
		return errors.New("invalid profile import rollback ID")
	}
	return store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		index := profileIndex(state.Profiles, name)
		if index < 0 {
			return fmt.Errorf(profileDoesNotExistFormat, name)
		}
		profile := state.Profiles[index]
		release, err := store.acquireMutation(name)
		if err != nil {
			return err
		}
		defer release()
		return prepareImportRollback(profile.CodexHome, id, paths)
	})
}

func (store *Store) RestoreBundleImportRollback(
	ctx context.Context,
	name, id string,
	before profilemodel.ImportLifecycleProfile,
) error {
	if !validImportID(id) {
		return errors.New("invalid profile import rollback ID")
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
		if before.CodexHome != state.Profiles[index].CodexHome {
			return errors.New("profile import rollback target changed")
		}
		if err := restoreImportRollback(before.CodexHome, id); err != nil {
			return err
		}
		state.Profiles[index] = importLifecycleProfile(before, name)
		return store.writeState(state)
	})
}

func (store *Store) CleanupBundleImportRollback(ctx context.Context, name, id string) error {
	if !validImportID(id) {
		return errors.New("invalid profile import rollback ID")
	}
	return store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		index := profileIndex(state.Profiles, name)
		if index < 0 {
			return nil
		}
		return removeImportRollback(filepath.Join(state.Profiles[index].CodexHome, bundleImportBackupDir+id))
	})
}

func (store *Store) RemoveBundleImportedProfile(ctx context.Context, action profilemodel.ImportLifecycleAction, id string) error {
	if !action.Create || profileentity.ValidateName(action.Name) != nil {
		return errors.New("invalid profile import creation rollback")
	}
	if !validImportID(id) {
		return errors.New("invalid profile import lifecycle ID")
	}
	name := action.Name
	expected := importLifecycleProfile(action.After, name)
	if !expected.Managed || filepath.Clean(expected.CodexHome) != filepath.Clean(store.ManagedHome(name)) {
		return errors.New("refusing to remove a non-imported profile home")
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
		if index >= 0 {
			if state.Profiles[index] != expected {
				return errors.New("refusing to remove a profile that does not match the import journal")
			}
			if err := verifyBundleImportHome(expected.CodexHome, action.Files, id); err != nil {
				return err
			}
			state.Profiles = append(state.Profiles[:index], state.Profiles[index+1:]...)
			repairActive(&state, name)
			if err := store.writeState(state); err != nil {
				return err
			}
		}
		return store.finishBundleImportHomeRemoval(name, id, action.Files)
	})
}

func (store *Store) finishBundleImportHomeRemoval(
	name, id string,
	files []profilemodel.ImportLifecycleFile,
) error {
	home := store.ManagedHome(name)
	removal := store.bundleImportRemovalPath(name, id)
	if _, err := os.Lstat(removal); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Lstat(home); err == nil {
			if err := verifyBundleImportHome(home, files, id); err != nil {
				return err
			}
			if err := os.Rename(home, removal); err != nil {
				return fmt.Errorf("quarantine imported profile home: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := removeManagedImportHome(removal); err != nil {
		return err
	}
	return fileutil.SyncDirectory(store.profilesRoot())
}

func verifyBundleImportHome(home string, files []profilemodel.ImportLifecycleFile, id string) error {
	if err := validateBundleImportHomeDirectory(home); err != nil {
		return err
	}
	want, err := bundleImportExpectedFiles(files)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != len(files)+1 {
		return errors.New("imported profile home does not match the import journal")
	}
	for _, entry := range entries {
		if err := verifyBundleImportHomeEntry(home, entry, want, id); err != nil {
			return err
		}
	}
	return nil
}

func validateBundleImportHomeDirectory(home string) error {
	info, err := os.Lstat(home)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("imported profile home is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return errors.New("imported profile home is not private")
	}
	return nil
}

func bundleImportExpectedFiles(files []profilemodel.ImportLifecycleFile) (map[string]string, error) {
	want := make(map[string]string, len(files))
	for _, file := range files {
		if err := validateImportRollbackFile(file.Path); err != nil || len(file.SHA256) != sha256.Size*2 {
			return nil, errors.New("invalid imported profile file journal")
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return nil, errors.New("invalid imported profile file journal")
		}
		if _, exists := want[file.Path]; exists {
			return nil, errors.New("duplicate imported profile file journal entry")
		}
		want[file.Path] = file.SHA256
	}
	return want, nil
}

func verifyBundleImportHomeEntry(home string, entry os.DirEntry, want map[string]string, id string) error {
	if isBundleImportOwnerFile(entry.Name(), id) {
		owned, err := readBundleImportOwner(home, id)
		if err != nil || !owned {
			return errors.New("profile import lifecycle marker does not match")
		}
		return nil
	}
	digestText, exists := want[entry.Name()]
	if !exists || entry.IsDir() {
		return errors.New("imported profile home does not match the import journal")
	}
	matches, err := bundleImportFileDigestMatches(filepath.Join(home, entry.Name()), digestText)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New("imported profile home does not match the import journal")
	}
	return nil
}

func bundleImportFileDigestMatches(path, digestText string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > providerSecretMaxBytes {
		return false, errors.New("imported profile file is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return false, errors.New("imported profile file is not private")
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > providerSecretMaxBytes {
		clearBytes(content)
		return false, errors.New("imported profile file is unavailable")
	}
	digest := sha256.Sum256(content)
	clearBytes(content)
	return strings.EqualFold(hex.EncodeToString(digest[:]), digestText), nil
}

func (store *Store) CheckBundleImportHomeAvailable(ctx context.Context, name string) error {
	if err := profileentity.ValidateName(name); err != nil {
		return err
	}
	return store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		if profileIndex(state.Profiles, name) >= 0 {
			return fmt.Errorf("profile %q already exists", name)
		}
		if _, err := os.Lstat(store.ManagedHome(name)); err == nil {
			return fmt.Errorf("managed profile home already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect managed profile home: %w", err)
		}
		return nil
	})
}

func (store *Store) CleanupOrphanedImportStagingHomes(ctx context.Context) error {
	return store.withLock(ctx, func() error {
		entries, err := os.ReadDir(store.profilesRoot())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, ".import-") && !strings.HasPrefix(name, ".provider-import-") {
				continue
			}
			path := filepath.Join(store.profilesRoot(), name)
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("unsafe orphaned profile import staging path %s", path)
			}
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("remove orphaned profile import staging home: %w", err)
			}
		}
		return nil
	})
}

func validImportID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (store *Store) bundleImportRemovalPath(name, id string) string {
	return filepath.Join(store.profilesRoot(), bundleImportRemovalDir+id+"-"+name)
}
