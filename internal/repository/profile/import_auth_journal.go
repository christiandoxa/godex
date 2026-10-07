package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

func (store *Store) writeProfileImportAuthJournal(journal profileImportAuthJournal) (bool, error) {
	content, err := json.Marshal(journal)
	if err != nil {
		return false, errors.New("encode profile import auth journal")
	}
	return fileutil.AtomicWrite(store.importAuthJournalPath(), content)
}

func (store *Store) replaceImportedAuthLocked(profile profileentity.Profile, authJSON []byte) error {
	homeInfo, err := os.Lstat(profile.CodexHome)
	if err != nil || homeInfo.Mode()&os.ModeSymlink != 0 || !homeInfo.IsDir() {
		return errors.New("profile CODEX_HOME must be a real directory")
	}
	authPath := filepath.Join(profile.CodexHome, profileAuthFileName)
	authMissing := false
	var previous []byte
	if _, err := os.Lstat(authPath); errors.Is(err, os.ErrNotExist) {
		authMissing = true
	} else if err != nil {
		return err
	} else {
		previous, err = store.ReadAuthJSON(profile.CodexHome)
		if err != nil {
			return err
		}
		defer clearBytes(previous)
	}

	backupPath := filepath.Join(profile.CodexHome, profileImportAuthBackupName)
	if _, err := os.Lstat(backupPath); !errors.Is(err, os.ErrNotExist) {
		return errors.New("profile import auth backup already exists")
	}
	journalPath := store.importAuthJournalPath()
	if _, err := os.Lstat(journalPath); !errors.Is(err, os.ErrNotExist) {
		return errors.New("profile import auth journal already exists")
	}
	journal := profileImportAuthJournal{
		Version: profileImportAuthJournalV1, Profile: profile.Name,
		CodexHome: profile.CodexHome, BackupName: profileImportAuthBackupName, Phase: "prepared",
		AuthMissing: authMissing,
	}
	if committed, err := store.writeProfileImportAuthJournal(journal); err != nil {
		if !committed {
			_ = os.Remove(journalPath)
		}
		return fmt.Errorf("create profile import auth journal: %w", err)
	}
	if !authMissing {
		if _, err := fileutil.AtomicWrite(backupPath, previous); err != nil {
			return fmt.Errorf("back up imported profile authentication: %w", err)
		}
	}
	journal.Phase = "backed_up"
	if _, err := store.writeProfileImportAuthJournal(journal); err != nil {
		return fmt.Errorf("record profile import auth backup: %w", err)
	}
	if _, err := fileutil.AtomicWrite(filepath.Join(profile.CodexHome, profileAuthFileName), authJSON); err != nil {
		return fmt.Errorf("replace imported profile authentication: %w", err)
	}
	journal.Phase = "committed"
	if _, err := store.writeProfileImportAuthJournal(journal); err != nil {
		return fmt.Errorf("commit imported profile authentication: %w", err)
	}
	return store.recoverProfileImportAuthJournal(journalPath, profile, journal)
}
