package profile

import (
	"context"
	"errors"
	"fmt"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func (catalog *Catalog) applyImport(ctx context.Context, plan importPlan) (profilemodel.ImportResult, error) {
	journal, err := catalog.prepareBundleImport(ctx, plan)
	if err != nil {
		return profilemodel.ImportResult{}, err
	}
	result := profilemodel.ImportResult{}
	for _, action := range plan.actions {
		if err := catalog.applyImportAction(ctx, action, journal.ID); err != nil {
			return profilemodel.ImportResult{}, catalog.abortBundleImport(ctx, journal, err)
		}
		if action.create {
			result.ImportedCount++
		} else {
			result.UpdatedCount++
		}
	}
	if err := catalog.finalizeImportActive(ctx, plan, &result); err != nil {
		return profilemodel.ImportResult{}, catalog.abortBundleImport(ctx, journal, err)
	}
	journal.Phase = "committed"
	if err := catalog.profiles.WriteBundleImportJournal(journal); err != nil {
		return profilemodel.ImportResult{}, fmt.Errorf("profile import commit marker failed; recovery is required: %w", err)
	}
	if err := catalog.cleanupBundleImportJournal(ctx, journal); err != nil {
		return result, fmt.Errorf("profile import committed but cleanup is pending: %w", err)
	}
	return result, nil
}

func (catalog *Catalog) applyImportAction(ctx context.Context, action importAction, journalID string) error {
	switch sourceProviderKind(action.source) {
	case profileentity.ProviderOpenAI:
		authBytes := []byte(action.source.AuthJSON)
		defer clearBundleBytes(authBytes)
		if action.create {
			return catalog.profiles.ImportBundleProfile(ctx, action.target.Profile, map[string][]byte{"auth.json": authBytes}, journalID)
		}
		return catalog.replaceImportedAuth(ctx, action.target, authBytes)
	case profileentity.ProviderAnthropic, profileentity.ProviderGemini, profileentity.ProviderKiro, profileentity.ProviderCopilot, profileentity.ProviderAgy:
		secrets, err := providerSecrets(action.source)
		if err != nil {
			return err
		}
		if action.create {
			files := make(map[string][]byte, len(secrets))
			defer func() {
				for _, content := range files {
					clearBundleBytes(content)
				}
			}()
			for name, secret := range secrets {
				files[name] = []byte(secret)
			}
			return catalog.profiles.ImportBundleProfile(ctx, action.target.Profile, files, journalID)
		}
		provider := providerFromSnapshot(action.source.Provider)
		provider.Kind = sourceProviderKind(action.source)
		return catalog.profiles.ReplaceProvider(ctx, action.target.Profile.Name,
			importedProfileEmail(action.source, action.identity), provider, secrets, false)
	default:
		return fmt.Errorf("profile provider %q import is not implemented yet", sourceProviderKind(action.source))
	}
}

func (catalog *Catalog) replaceImportedAuth(ctx context.Context, target Report, authJSON []byte) error {
	if target.AccountID != "" {
		return catalog.accounts.ReplaceImportedAuth(ctx, target.AccountID, authJSON)
	}
	return catalog.profiles.ReplaceAuth(ctx, target.Profile.Name, authJSON)
}

func (catalog *Catalog) finalizeImportActive(ctx context.Context, plan importPlan, result *profilemodel.ImportResult) error {
	if plan.activeTarget != "" {
		if _, err := catalog.use(ctx, plan.activeTarget); err != nil {
			return err
		}
		result.ActiveProfile = plan.activeTarget
		return nil
	}
	if !plan.keepActive {
		return nil
	}
	listed, err := catalog.list(ctx)
	if err != nil {
		return err
	}
	for _, report := range listed {
		if report.Active {
			result.ActiveProfile = report.Profile.Name
			break
		}
	}
	return nil
}

func (catalog *Catalog) abortBundleImport(ctx context.Context, journal profilemodel.ImportLifecycleJournal, cause error) error {
	return errors.Join(cause, catalog.recoverBundleImportLocked(context.WithoutCancel(ctx), journal))
}
