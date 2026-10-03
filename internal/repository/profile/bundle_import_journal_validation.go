package profile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func validateBundleImportJournal(store *Store, journal profilemodel.ImportLifecycleJournal) error {
	if err := validateBundleImportJournalHeader(journal); err != nil {
		return err
	}
	seen := make(map[string]bool, len(journal.Actions))
	for _, action := range journal.Actions {
		if err := validateBundleImportAction(store, journal.ID, action, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateBundleImportJournalHeader(journal profilemodel.ImportLifecycleJournal) error {
	if journal.Version != bundleImportVersion || !validImportID(journal.ID) || len(journal.Actions) > 256 {
		return errors.New("invalid profile import lifecycle journal")
	}
	switch journal.Phase {
	case "preparing", "applying", "committed", "rolled_back":
		return nil
	default:
		return errors.New("invalid profile import lifecycle journal phase")
	}
}

func validateBundleImportAction(
	store *Store,
	journalID string,
	action profilemodel.ImportLifecycleAction,
	seen map[string]bool,
) error {
	if err := profileentity.ValidateName(action.Name); err != nil || seen[action.Name] {
		return errors.New("invalid profile import lifecycle target")
	}
	seen[action.Name] = true
	if err := profileentity.Validate(importLifecycleProfile(action.After, action.Name)); err != nil {
		return errors.New("invalid profile import lifecycle after-state")
	}
	if err := validateBundleImportActionShape(store, journalID, action); err != nil {
		return err
	}
	return validateBundleImportActionFiles(action)
}

func validateBundleImportActionShape(store *Store, journalID string, action profilemodel.ImportLifecycleAction) error {
	switch {
	case action.Create:
		if action.Before != nil || action.AccountID != "" || action.BackupID != "" ||
			!action.After.Managed ||
			filepath.Clean(action.After.CodexHome) != filepath.Clean(store.ManagedHome(action.Name)) {
			return errors.New("invalid profile import creation journal")
		}
		return nil
	case action.AccountID == "":
		if action.Before == nil || !validImportID(action.BackupID) || action.BackupID != journalID {
			return errors.New("invalid profile import update journal")
		}
		before := importLifecycleProfile(*action.Before, action.Name)
		if err := profileentity.Validate(before); err != nil ||
			filepath.Clean(before.CodexHome) != filepath.Clean(action.After.CodexHome) {
			return errors.New("invalid profile import lifecycle before-state")
		}
		return nil
	default:
		return validateBundleImportAccountAction(journalID, action)
	}
}

func validateBundleImportAccountAction(journalID string, action profilemodel.ImportLifecycleAction) error {
	if action.Before != nil || !validImportID(action.BackupID) || action.BackupID != journalID || len(action.AccountID) != 32 {
		return errors.New("invalid account import update journal")
	}
	if _, err := hex.DecodeString(action.AccountID); err != nil {
		return errors.New("invalid account import update journal")
	}
	if len(action.Files) != 1 || action.Files[0].Path != profileAuthFileName {
		return errors.New("invalid account import auth journal")
	}
	return nil
}

func validateBundleImportActionFiles(action profilemodel.ImportLifecycleAction) error {
	if len(action.Files) > 16 {
		return errors.New("profile import journal has too many secret files")
	}
	seen := make(map[string]bool, len(action.Files))
	for _, file := range action.Files {
		if err := validateBundleImportLifecycleFile(file, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateBundleImportLifecycleFile(file profilemodel.ImportLifecycleFile, seen map[string]bool) error {
	if err := validateImportRollbackFile(file.Path); err != nil || len(file.SHA256) != sha256.Size*2 {
		return errors.New("invalid profile import lifecycle file entry")
	}
	if seen[file.Path] {
		return errors.New("duplicate profile import lifecycle file entry")
	}
	seen[file.Path] = true
	if _, err := hex.DecodeString(file.SHA256); err != nil {
		return errors.New("invalid profile import lifecycle file digest")
	}
	return nil
}

func readBundleImportJournal(store *Store, path string) (profilemodel.ImportLifecycleJournal, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > bundleImportJournalMax || info.Mode()&os.ModeSymlink != 0 {
		return profilemodel.ImportLifecycleJournal{}, errors.New("profile import lifecycle journal is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return profilemodel.ImportLifecycleJournal{}, errors.New("profile import lifecycle journal is not private")
	}
	file, err := os.Open(path)
	if err != nil {
		return profilemodel.ImportLifecycleJournal{}, errors.New("profile import lifecycle journal is unavailable")
	}
	content, readErr := io.ReadAll(io.LimitReader(file, bundleImportJournalMax+1))
	err = errors.Join(readErr, file.Close())
	if err != nil || len(content) > bundleImportJournalMax {
		return profilemodel.ImportLifecycleJournal{}, errors.New("invalid profile import lifecycle journal")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var journal profilemodel.ImportLifecycleJournal
	if err := decoder.Decode(&journal); err != nil || requireBundleJournalEOF(decoder) != nil {
		return profilemodel.ImportLifecycleJournal{}, errors.New("decode profile import lifecycle journal")
	}
	if err := validateBundleImportJournal(store, journal); err != nil {
		return profilemodel.ImportLifecycleJournal{}, err
	}
	return journal, nil
}

func (store *Store) bundleImportJournalRoot() string {
	return filepath.Join(store.root, bundleImportJournalDir)
}

func (store *Store) bundleImportJournalPath(id string) string {
	return filepath.Join(store.bundleImportJournalRoot(), id+".json")
}

func requireBundleJournalEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("profile import lifecycle journal has trailing data")
	}
	return nil
}

func importLifecycleProfile(value profilemodel.ImportLifecycleProfile, name string) profileentity.Profile {
	return profileentity.Profile{
		Name: name, CodexHome: value.CodexHome, Managed: value.Managed,
		Email: value.Email, Provider: providerFromLifecycleSnapshot(value.Provider),
	}
}

func providerFromLifecycleSnapshot(value profilemodel.ProviderSnapshot) profileentity.Provider {
	return profileentity.Provider{
		Kind:      profileentity.ProviderKind(value.Kind),
		ProjectID: lifecycleString(value.ProjectID), Account: lifecycleString(value.Account),
		AuthMethod: lifecycleString(value.AuthMethod), Host: lifecycleString(value.Host),
		Login: lifecycleString(value.Login), APIURL: lifecycleString(value.APIURL),
		AccessTypeSKU: lifecycleString(value.AccessTypeSKU), CopilotPlan: lifecycleString(value.CopilotPlan),
		AuthKey: lifecycleString(value.AuthKey), AuthKind: lifecycleString(value.AuthKind),
		ProfileARN: lifecycleString(value.ProfileARN), ProfileName: lifecycleString(value.ProfileName),
		StartURL: lifecycleString(value.StartURL), Region: lifecycleString(value.Region),
	}
}

func lifecycleString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
