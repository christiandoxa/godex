package profile

import (
	"bytes"
	"context"
	"encoding/json"
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
	profileImportAuthJournalName = ".profile-import-auth-journal.json"
	profileImportAuthBackupName  = ".profile-import-auth-backup"
	profileImportAuthJournalMax  = 16 << 10
	profileImportAuthJournalV1   = 1
)

type profileImportAuthJournal struct {
	Version    int    `json:"version"`
	Profile    string `json:"profile"`
	CodexHome  string `json:"codex_home"`
	BackupName string `json:"backup_name"`
	Phase      string `json:"phase"`
}

func (store *Store) RepairImportAuthJournals(ctx context.Context) (int, error) {
	recovered := 0
	err := store.withLockRecovering(ctx, func(count int) error {
		recovered = count
		return nil
	})
	return recovered, err
}

func (store *Store) recoverImportAuthJournalLocked() (int, error) {
	path := store.importAuthJournalPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("inspect profile import auth journal: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > profileImportAuthJournalMax {
		return 0, errors.New("invalid profile import auth journal")
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("read profile import auth journal: %w", err)
	}
	content, readErr := io.ReadAll(io.LimitReader(file, profileImportAuthJournalMax+1))
	err = errors.Join(readErr, file.Close())
	if err != nil || len(content) > profileImportAuthJournalMax {
		return 0, errors.New("invalid profile import auth journal")
	}
	journal, err := decodeProfileImportAuthJournal(content)
	if err != nil {
		return 0, err
	}
	state, err := store.readState()
	if err != nil {
		return 0, err
	}
	profile, err := store.validateImportAuthJournal(journal, state)
	if err != nil {
		return 0, err
	}
	release, err := store.acquireMutation(profile.Name)
	if err != nil {
		return 0, err
	}
	defer release()
	if err := store.recoverProfileImportAuthJournal(path, profile, journal); err != nil {
		return 0, err
	}
	return 1, nil
}

func decodeProfileImportAuthJournal(content []byte) (profileImportAuthJournal, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var journal profileImportAuthJournal
	if err := decoder.Decode(&journal); err != nil {
		return profileImportAuthJournal{}, errors.New("decode profile import auth journal")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return profileImportAuthJournal{}, errors.New("profile import auth journal has trailing data")
	}
	if journal.Version != profileImportAuthJournalV1 || journal.BackupName != profileImportAuthBackupName {
		return profileImportAuthJournal{}, errors.New("unsupported profile import auth journal")
	}
	switch journal.Phase {
	case "prepared", "backed_up", "rolled_back", "committed":
		return journal, nil
	default:
		return profileImportAuthJournal{}, errors.New("invalid profile import auth journal phase")
	}
}

func (store *Store) validateImportAuthJournal(
	journal profileImportAuthJournal,
	state stateFile,
) (profileentity.Profile, error) {
	if err := profileentity.ValidateName(journal.Profile); err != nil {
		return profileentity.Profile{}, errors.New("invalid profile import auth journal target")
	}
	index := profileIndex(state.Profiles, journal.Profile)
	if index < 0 || state.Profiles[index].CodexHome != journal.CodexHome {
		return profileentity.Profile{}, errors.New("profile import auth journal target changed")
	}
	profile := state.Profiles[index]
	info, err := os.Lstat(profile.CodexHome)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return profileentity.Profile{}, errors.New("profile import auth journal home is unavailable")
	}
	return profile, nil
}

func (store *Store) recoverProfileImportAuthJournal(
	journalPath string,
	profile profileentity.Profile,
	journal profileImportAuthJournal,
) error {
	backupPath := filepath.Join(profile.CodexHome, journal.BackupName)
	switch journal.Phase {
	case "prepared":
		if err := removeProfileImportAuthBackup(backupPath, true); err != nil {
			return err
		}
	case "backed_up":
		backup, err := readProfileImportAuthBackup(backupPath)
		if err != nil {
			return err
		}
		defer clearBytes(backup)
		authPath := filepath.Join(profile.CodexHome, profileAuthFileName)
		if err := validateProfileAuthTarget(authPath); err != nil {
			return err
		}
		if _, err := fileutil.AtomicWrite(authPath, backup); err != nil {
			return fmt.Errorf("restore imported profile authentication: %w", err)
		}
		journal.Phase = "rolled_back"
		if _, err := store.writeProfileImportAuthJournal(journal); err != nil {
			return fmt.Errorf("mark imported profile authentication rollback: %w", err)
		}
		fallthrough
	case "rolled_back", "committed":
		if err := removeProfileImportAuthBackup(backupPath, true); err != nil {
			return err
		}
	}
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove profile import auth journal: %w", err)
	}
	if err := fileutil.SyncDirectory(store.root); err != nil {
		return fmt.Errorf("sync profile import auth journal cleanup: %w", err)
	}
	return nil
}

func readProfileImportAuthBackup(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxProfileAuthBytes {
		return nil, errors.New("profile import auth backup is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("profile import auth backup is not private")
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > maxProfileAuthBytes {
		clearBytes(content)
		return nil, errors.New("profile import auth backup is unavailable")
	}
	return content, nil
}

func validateProfileAuthTarget(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxProfileAuthBytes {
		return errors.New("profile authentication target is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return errors.New("profile authentication target is not private")
	}
	return nil
}

func removeProfileImportAuthBackup(path string, missingOK bool) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && missingOK {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect profile import auth backup: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("profile import auth backup is not a regular file")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove profile import auth backup: %w", err)
	}
	if err := fileutil.SyncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync profile import auth backup cleanup: %w", err)
	}
	return nil
}

func (store *Store) importAuthJournalPath() string {
	return filepath.Join(store.root, profileImportAuthJournalName)
}
