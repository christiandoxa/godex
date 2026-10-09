package profile

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

type bundleImportCommitVerifier interface {
	BundleImportActionCommitted(context.Context, profilemodel.ImportLifecycleAction, string) (bool, error)
}

type importedAuthCommitVerifier interface {
	ImportedAuthMatches(context.Context, string, string) (bool, error)
}

type selectedLoginCommitVerifier interface {
	SelectedLoginActionCommitted(context.Context, profilemodel.ImportLifecycleAction) (bool, error)
}

func (catalog *Catalog) withBundleImportLock(ctx context.Context, operation func() error) (err error) {
	release, err := catalog.profiles.AcquireBundleImportLock(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	recoveryCtx := context.WithoutCancel(ctx)
	// A journal is evidence of an already-started mutation. Finish its
	// rollback/cleanup even when the new command's context was canceled.
	if err = catalog.recoverBundleImportsLocked(recoveryCtx); err != nil {
		return err
	}
	if err = catalog.profiles.CleanupOrphanedImportStagingHomes(recoveryCtx); err != nil {
		return err
	}
	return operation()
}

func (catalog *Catalog) recoverBundleImportsLocked(ctx context.Context) error {
	journals, err := catalog.profiles.BundleImportJournals()
	if err != nil {
		return err
	}
	for _, journal := range journals {
		if err := catalog.recoverBundleImportLocked(ctx, journal); err != nil {
			return err
		}
	}
	return nil
}

func (catalog *Catalog) recoverBundleImportLocked(ctx context.Context, journal profilemodel.ImportLifecycleJournal) error {
	switch journal.Phase {
	case "preparing", "committed", "rolled_back":
		return catalog.cleanupBundleImportJournal(ctx, journal)
	case "applying":
		committed, err := catalog.bundleImportCommitted(ctx, journal)
		if err != nil {
			return err
		}
		if !committed {
			return catalog.rollbackBundleImport(ctx, journal)
		}
		journal.Phase = "committed"
		if err := catalog.profiles.WriteBundleImportJournal(journal); err != nil {
			return fmt.Errorf("mark recovered profile import committed: %w", err)
		}
		return catalog.cleanupBundleImportJournal(ctx, journal)
	default:
		return fmt.Errorf("unsupported profile import lifecycle phase %q", journal.Phase)
	}
}

func (catalog *Catalog) bundleImportCommitted(ctx context.Context, journal profilemodel.ImportLifecycleJournal) (bool, error) {
	for _, action := range journal.Actions {
		committed, err := catalog.bundleImportActionCommitted(ctx, action, journal.ID)
		if err != nil || !committed {
			return false, err
		}
	}
	return catalog.bundleImportSelectionCommitted(ctx, journal)
}

func (catalog *Catalog) bundleImportActionCommitted(
	ctx context.Context,
	action profilemodel.ImportLifecycleAction,
	journalID string,
) (bool, error) {
	if action.AccountID != "" {
		if verifier, ok := catalog.accounts.(selectedLoginCommitVerifier); ok {
			return verifier.SelectedLoginActionCommitted(ctx, action)
		}
		verifier, ok := catalog.accounts.(importedAuthCommitVerifier)
		if !ok || len(action.Files) != 1 {
			return false, nil
		}
		return verifier.ImportedAuthMatches(ctx, action.AccountID, action.Files[0].SHA256)
	}
	verifier, ok := catalog.profiles.(bundleImportCommitVerifier)
	if !ok {
		return false, nil
	}
	return verifier.BundleImportActionCommitted(ctx, action, journalID)
}

func (catalog *Catalog) bundleImportSelectionCommitted(ctx context.Context, journal profilemodel.ImportLifecycleJournal) (bool, error) {
	expectedProfile := journal.PreviousProfileActive
	expectedAccount := journal.PreviousAccountActive
	if journal.NextAccountActive != "" {
		expectedProfile = ""
		expectedAccount = journal.NextAccountActive
	} else if journal.NextProfileActive != "" {
		expectedProfile = journal.NextProfileActive
	}

	actualProfile := ""
	hasProfile, err := catalog.profiles.HasActive(ctx)
	if err != nil {
		return false, err
	}
	if hasProfile {
		profile, err := catalog.profiles.Current(ctx)
		if err != nil {
			return false, err
		}
		actualProfile = profile.Name
	}
	actualAccount, err := catalog.accounts.ActiveID(ctx)
	if err != nil {
		return false, err
	}
	return actualProfile == expectedProfile && actualAccount == expectedAccount, nil
}

func (catalog *Catalog) rollbackBundleImport(ctx context.Context, journal profilemodel.ImportLifecycleJournal) error {
	for index := len(journal.Actions) - 1; index >= 0; index-- {
		if err := catalog.rollbackBundleImportAction(ctx, journal.Actions[index], journal.ID); err != nil {
			return err
		}
	}
	if err := catalog.restoreBundleImportSelection(ctx, journal); err != nil {
		return err
	}
	journal.Phase = "rolled_back"
	if err := catalog.profiles.WriteBundleImportJournal(journal); err != nil {
		return err
	}
	return catalog.cleanupBundleImportJournal(ctx, journal)
}

func (catalog *Catalog) rollbackBundleImportAction(ctx context.Context, action profilemodel.ImportLifecycleAction, journalID string) error {
	if action.Create {
		return catalog.profiles.RemoveBundleImportedProfile(ctx, action, journalID)
	}
	if action.AccountID != "" {
		return catalog.accounts.RestoreImportedAuthRollback(ctx, action.AccountID, action.BackupID)
	}
	if action.Before == nil {
		return errors.New("profile import rollback state is missing")
	}
	return catalog.profiles.RestoreBundleImportRollback(ctx, action.Name, action.BackupID, *action.Before)
}

func (catalog *Catalog) restoreBundleImportSelection(ctx context.Context, journal profilemodel.ImportLifecycleJournal) error {
	if journal.PreviousProfileActive == "" {
		if err := catalog.profiles.ClearActive(ctx); err != nil {
			return err
		}
	} else if _, err := catalog.profiles.SetActive(ctx, journal.PreviousProfileActive); err != nil {
		return err
	}
	if journal.PreviousAccountActive == "" {
		return catalog.accounts.ClearActive(ctx)
	}
	_, err := catalog.accounts.SetActive(ctx, journal.PreviousAccountActive)
	return err
}

func (catalog *Catalog) cleanupBundleImportJournal(ctx context.Context, journal profilemodel.ImportLifecycleJournal) error {
	var cleanupErr error
	for _, action := range journal.Actions {
		if action.Create {
			cleanupErr = errors.Join(cleanupErr, catalog.profiles.CleanupBundleImportOwnerMarker(ctx, action.Name, journal.ID))
			continue
		}
		if action.AccountID != "" {
			cleanupErr = errors.Join(cleanupErr, catalog.accounts.CleanupImportedAuthRollback(ctx, action.AccountID, action.BackupID))
		} else {
			cleanupErr = errors.Join(cleanupErr, catalog.profiles.CleanupBundleImportRollback(ctx, action.Name, action.BackupID))
		}
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	return catalog.profiles.RemoveBundleImportJournal(journal.ID)
}

func (catalog *Catalog) prepareBundleImport(ctx context.Context, plan importPlan) (profilemodel.ImportLifecycleJournal, error) {
	for _, action := range plan.actions {
		if action.create {
			if err := catalog.profiles.CheckBundleImportHomeAvailable(ctx, action.target.Profile.Name); err != nil {
				return profilemodel.ImportLifecycleJournal{}, err
			}
		}
	}
	journal, err := catalog.newBundleImportJournal(ctx, plan)
	if err != nil {
		return profilemodel.ImportLifecycleJournal{}, err
	}
	if err := catalog.profiles.WriteBundleImportJournal(journal); err != nil {
		return profilemodel.ImportLifecycleJournal{}, err
	}
	for _, action := range journal.Actions {
		if action.Create {
			continue
		}
		if action.AccountID != "" {
			err = catalog.accounts.PrepareImportedAuthRollback(ctx, action.AccountID, journal.ID)
		} else {
			err = catalog.profiles.PrepareBundleImportRollback(ctx, action.Name, journal.ID, importFileNames(action.Files))
		}
		if err != nil {
			return profilemodel.ImportLifecycleJournal{}, errors.Join(err, catalog.cleanupBundleImportJournal(ctx, journal))
		}
	}
	journal.Phase = "applying"
	if err := catalog.profiles.WriteBundleImportJournal(journal); err != nil {
		return profilemodel.ImportLifecycleJournal{}, fmt.Errorf("write profile import applying journal: %w", err)
	}
	return journal, nil
}

func (catalog *Catalog) newBundleImportJournal(ctx context.Context, plan importPlan) (profilemodel.ImportLifecycleJournal, error) {
	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return profilemodel.ImportLifecycleJournal{}, fmt.Errorf("create profile import lifecycle ID: %w", err)
	}
	journal := profilemodel.ImportLifecycleJournal{Version: 1, ID: hex.EncodeToString(randomID[:]), Phase: "preparing"}
	hasProfile, err := catalog.profiles.HasActive(ctx)
	if err != nil {
		return profilemodel.ImportLifecycleJournal{}, err
	}
	if hasProfile {
		active, err := catalog.profiles.Current(ctx)
		if err != nil {
			return profilemodel.ImportLifecycleJournal{}, err
		}
		journal.PreviousProfileActive = active.Name
	}
	journal.PreviousAccountActive, err = catalog.accounts.ActiveID(ctx)
	if err != nil {
		return profilemodel.ImportLifecycleJournal{}, err
	}
	for _, action := range plan.actions {
		lifecycleAction := profilemodel.ImportLifecycleAction{
			Name: action.target.Profile.Name, Create: action.create,
			After: importLifecycleProfile(actionAfterProfile(action)),
			Files: importActionLifecycleFiles(action), IdentityCleared: action.identityCleared,
		}
		if action.create {
			lifecycleAction.After = importLifecycleProfile(action.target.Profile)
		} else if action.target.AccountID != "" {
			lifecycleAction.AccountID = action.target.AccountID
			lifecycleAction.BackupID = journal.ID
		} else {
			before := importLifecycleProfile(action.target.Profile)
			lifecycleAction.Before = &before
			lifecycleAction.BackupID = journal.ID
		}
		journal.Actions = append(journal.Actions, lifecycleAction)
		if plan.activeTarget == action.target.Profile.Name {
			if action.target.AccountID != "" {
				journal.NextAccountActive = action.target.AccountID
			} else {
				journal.NextProfileActive = action.target.Profile.Name
			}
		}
	}
	return journal, nil
}

func actionAfterProfile(action importAction) profileentity.Profile {
	if action.after != nil {
		return *action.after
	}
	profile := action.target.Profile
	if !action.create && sourceProviderKind(action.source) != profileentity.ProviderOpenAI {
		profile.Email = importedProfileEmail(action.source, action.identity)
		profile.Provider = providerFromSnapshot(action.source.Provider)
		profile.Provider.Kind = sourceProviderKind(action.source)
	}
	return profile
}

func importLifecycleProfile(profile profileentity.Profile) profilemodel.ImportLifecycleProfile {
	return profilemodel.ImportLifecycleProfile{
		CodexHome: profile.CodexHome, Managed: profile.Managed, Email: profile.Email,
		Provider: providerSnapshotFromEntity(profile.Provider),
	}
}

func importActionLifecycleFiles(action importAction) []profilemodel.ImportLifecycleFile {
	result := importLifecycleFiles(action.source)
	for _, file := range action.extraFiles {
		content := []byte(file.Text)
		digest := sha256.Sum256(content)
		clearBundleBytes(content)
		result = append(result, profilemodel.ImportLifecycleFile{Path: file.Path, SHA256: hex.EncodeToString(digest[:])})
	}
	for _, path := range action.removeFiles {
		result = append(result, profilemodel.ImportLifecycleFile{Path: path, Missing: true})
	}
	return result
}

func importLifecycleFiles(source profilemodel.ExportedProfile) []profilemodel.ImportLifecycleFile {
	var files []profilemodel.ExportedSecretFile
	if sourceProviderKind(source) == profileentity.ProviderOpenAI {
		files = []profilemodel.ExportedSecretFile{{Path: "auth.json", Text: source.AuthJSON}}
	} else {
		files = source.SecretFiles
	}
	result := make([]profilemodel.ImportLifecycleFile, 0, len(files))
	for _, file := range files {
		content := []byte(file.Text)
		digest := sha256.Sum256(content)
		clearBundleBytes(content)
		result = append(result, profilemodel.ImportLifecycleFile{Path: file.Path, SHA256: hex.EncodeToString(digest[:])})
	}
	return result
}

func importFileNames(files []profilemodel.ImportLifecycleFile) []string {
	names := make([]string, len(files))
	for index, file := range files {
		names[index] = file.Path
	}
	return names
}
