package profile

import (
	"context"
	"errors"
	"fmt"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

type importMutation struct {
	imported int
	updated  int
	rollback func() error
	cleanup  func()
}

func (catalog *Catalog) applyImport(ctx context.Context, plan importPlan) (profilemodel.ImportResult, error) {
	mutations := make([]importMutation, 0, len(plan.actions))
	result := profilemodel.ImportResult{}
	for _, action := range plan.actions {
		mutation, err := catalog.applyImportAction(ctx, action)
		if err != nil {
			return profilemodel.ImportResult{}, rollbackImport(mutations, err)
		}
		mutations = append(mutations, mutation)
		result.ImportedCount += mutation.imported
		result.UpdatedCount += mutation.updated
	}
	if err := catalog.finalizeImportActive(ctx, plan, &result); err != nil {
		return profilemodel.ImportResult{}, rollbackImport(mutations, err)
	}
	cleanupImport(mutations)
	return result, nil
}

func (catalog *Catalog) applyImportAction(ctx context.Context, action importAction) (importMutation, error) {
	switch sourceProviderKind(action.source) {
	case profileentity.ProviderOpenAI:
		return catalog.applyOpenAIImportAction(ctx, action)
	case profileentity.ProviderAnthropic, profileentity.ProviderKiro, profileentity.ProviderCopilot:
		return catalog.applyProviderImportAction(ctx, action)
	default:
		return importMutation{}, fmt.Errorf("profile provider %q import is not implemented yet", sourceProviderKind(action.source))
	}
}

func (catalog *Catalog) applyOpenAIImportAction(ctx context.Context, action importAction) (importMutation, error) {
	authBytes := []byte(action.source.AuthJSON)
	defer clearBundleBytes(authBytes)
	if action.create {
		if err := catalog.profiles.ImportOpenAI(ctx, action.target.Profile, authBytes, false); err != nil {
			return importMutation{}, err
		}
		return newProfileImportMutation(ctx, catalog, action.target.Profile.Name), nil
	}
	return catalog.updateImportedAuth(ctx, action.target, authBytes)
}

func (catalog *Catalog) applyProviderImportAction(ctx context.Context, action importAction) (importMutation, error) {
	secrets, err := providerSecrets(action.source)
	if err != nil {
		return importMutation{}, err
	}
	if action.create {
		if err := catalog.profiles.ImportProvider(ctx, action.target.Profile, secrets, false); err != nil {
			return importMutation{}, err
		}
		return newProfileImportMutation(ctx, catalog, action.target.Profile.Name), nil
	}
	return catalog.updateImportedProvider(ctx, action.target, action.source, secrets)
}

func newProfileImportMutation(ctx context.Context, catalog *Catalog, name string) importMutation {
	return importMutation{imported: 1, rollback: func() error {
		_, err := catalog.profiles.Remove(context.WithoutCancel(ctx), name, true)
		return err
	}}
}

func (catalog *Catalog) updateImportedAuth(ctx context.Context, target Report, authBytes []byte) (importMutation, error) {
	previous, err := catalog.profiles.ReadAuthJSON(target.Profile.CodexHome)
	if err != nil {
		return importMutation{}, err
	}
	if err := catalog.replaceImportedAuth(ctx, target, authBytes); err != nil {
		clearBundleBytes(previous)
		return importMutation{}, err
	}
	backup := append([]byte(nil), previous...)
	clearBundleBytes(previous)
	return importMutation{
		updated: 1,
		rollback: func() error {
			defer clearBundleBytes(backup)
			return catalog.replaceImportedAuth(context.WithoutCancel(ctx), target, backup)
		},
		cleanup: func() { clearBundleBytes(backup) },
	}, nil
}

func (catalog *Catalog) updateImportedProvider(
	ctx context.Context,
	target Report,
	source profilemodel.ExportedProfile,
	secrets map[string]string,
) (importMutation, error) {
	previousProfile := target.Profile
	previousSecrets, err := catalog.snapshotProviderSecrets(target.Profile.CodexHome, secrets)
	if err != nil {
		return importMutation{}, err
	}
	provider := providerFromSnapshot(source.Provider)
	provider.Kind = sourceProviderKind(source)
	email := importedProfileEmail(source, accountentity.Identity{})
	if err := catalog.profiles.ReplaceProvider(ctx, target.Profile.Name, email, provider, secrets, false); err != nil {
		return importMutation{}, err
	}
	return importMutation{
		updated: 1,
		rollback: func() error {
			return catalog.profiles.RestoreProvider(
				context.WithoutCancel(ctx), previousProfile.Name, previousProfile.Email, previousProfile.Provider, previousSecrets,
			)
		},
		cleanup: func() { clearProviderSecretSnapshot(previousSecrets) },
	}, nil
}

func (catalog *Catalog) snapshotProviderSecrets(codexHome string, next map[string]string) (map[string]*string, error) {
	previous := make(map[string]*string, len(next))
	for name := range next {
		text, found, err := catalog.profiles.ReadOptionalProviderSecret(codexHome, name)
		if err != nil {
			return nil, err
		}
		if !found {
			previous[name] = nil
			continue
		}
		copy := text
		previous[name] = &copy
	}
	return previous, nil
}

func clearProviderSecretSnapshot(values map[string]*string) {
	for _, value := range values {
		if value != nil {
			*value = ""
		}
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
		if _, err := catalog.Use(ctx, plan.activeTarget); err != nil {
			return err
		}
		result.ActiveProfile = plan.activeTarget
		return nil
	}
	if !plan.keepActive {
		return nil
	}
	listed, err := catalog.List(ctx)
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

func rollbackImport(mutations []importMutation, cause error) error {
	var rollbackErr error
	for index := len(mutations) - 1; index >= 0; index-- {
		if mutations[index].rollback != nil {
			rollbackErr = errors.Join(rollbackErr, mutations[index].rollback())
		}
	}
	return errors.Join(cause, rollbackErr)
}

func cleanupImport(mutations []importMutation) {
	for _, mutation := range mutations {
		if mutation.cleanup != nil {
			mutation.cleanup()
		}
	}
}
