package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	"github.com/pelletier/go-toml/v2"
)

func (catalog *Catalog) SelectedOpenAILogout(
	ctx context.Context,
	name string,
	run func(string) error,
) error {
	if run == nil {
		return errors.New("selected logout runner is not configured")
	}
	return catalog.withBundleImportLock(ctx, func() error {
		var target Report
		var err error
		if strings.TrimSpace(name) == "" {
			target, err = catalog.current(ctx)
		} else {
			target, err = catalog.selectedOpenAITargetLocked(ctx, name)
		}
		if err != nil {
			return err
		}
		if target.Profile.Provider.Kind != profileentity.ProviderOpenAI {
			return fmt.Errorf(
				"profile %q uses %s; godex logout currently supports OpenAI/Codex profiles only",
				target.Profile.Name, target.Profile.Provider.Kind,
			)
		}
		return run(target.Profile.CodexHome)
	})
}

func (catalog *Catalog) SelectedLoginStatus(
	ctx context.Context,
	name string,
	run func(string) error,
) error {
	if run == nil {
		return errors.New("selected login status runner is not configured")
	}
	return catalog.withBundleImportLock(ctx, func() error {
		target, err := catalog.selectedOpenAITargetLocked(ctx, name)
		if err != nil {
			return err
		}
		return run(target.Profile.CodexHome)
	})
}

func (catalog *Catalog) SelectedOpenAILogin(
	ctx context.Context,
	name string,
	login func() ([]byte, error),
) (Report, error) {
	if login == nil {
		return Report{}, errors.New("selected login runner is not configured")
	}
	var result Report
	err := catalog.withBundleImportLock(ctx, func() error {
		target, err := catalog.selectedOpenAITargetLocked(ctx, name)
		if err != nil {
			return err
		}
		authJSON, err := login()
		if err != nil {
			return err
		}
		defer clearBundleBytes(authJSON)
		if catalog.auth == nil {
			return errors.New("profile auth inspection is not configured")
		}
		identity, err := catalog.auth.InspectAuthJSON(ctx, authJSON)
		if err != nil {
			return errors.New("selected login produced invalid OpenAI authentication")
		}
		current, err := catalog.selectedOpenAITargetLocked(ctx, name)
		if err != nil || current.AccountID != target.AccountID || current.Profile != target.Profile {
			return fmt.Errorf("profile %q changed while login was running", name)
		}

		desired := target.Profile
		if email := strings.TrimSpace(identity.Email); email != "" {
			desired.Email = email
		}
		source := profilemodel.ExportedProfile{
			Name: target.Profile.Name, SourceManaged: target.Profile.Managed,
			Provider: providerSnapshotFromEntity(target.Profile.Provider), AuthJSON: string(authJSON),
		}
		action := importAction{source: source, target: target, identity: identity, after: &desired}
		plan := importPlan{
			actions: []importAction{action}, resolvedNames: map[string]string{name: name},
			activeTarget: name,
		}
		if _, err := catalog.applyImport(ctx, plan); err != nil {
			return err
		}
		result = Report{Profile: desired, Active: true, Enabled: target.Enabled, AccountID: target.AccountID}
		return nil
	})
	return result, err
}

func (catalog *Catalog) SelectedOpenAIAPIKey(
	ctx context.Context,
	name string,
	input profilemodel.APIKeyLoginInput,
) (Report, error) {
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return Report{}, errors.New("API key cannot be empty")
	}
	_, baseURLPointer, err := normalizeAPIKeyBaseURL(input.BaseURL, input.BaseURLSpecified)
	if err != nil {
		return Report{}, err
	}
	authJSON, err := apiKeyAuthJSON(apiKey)
	if err != nil {
		return Report{}, err
	}
	defer clearBundleBytes(authJSON)
	extraFiles, removeFiles, err := selectedAPIKeyConfigFiles(baseURLPointer, input.BaseURLSpecified)
	if err != nil {
		return Report{}, err
	}

	var result Report
	err = catalog.withBundleImportLock(ctx, func() error {
		target, err := catalog.selectedOpenAITargetLocked(ctx, name)
		if err != nil {
			return err
		}
		desired := target.Profile
		desired.Email = ""
		source := profilemodel.ExportedProfile{
			Name: target.Profile.Name, SourceManaged: target.Profile.Managed,
			Provider: providerSnapshotFromEntity(target.Profile.Provider), AuthJSON: string(authJSON),
		}
		action := importAction{
			source: source, target: target, after: &desired, extraFiles: extraFiles, removeFiles: removeFiles,
			identityCleared: target.AccountID != "", selectedAPIKey: true,
		}
		plan := importPlan{
			actions: []importAction{action}, resolvedNames: map[string]string{name: name},
			activeTarget: name,
		}
		if _, err := catalog.applyImport(ctx, plan); err != nil {
			return err
		}
		result = Report{Profile: desired, Active: true, Enabled: target.Enabled, AccountID: target.AccountID}
		return nil
	})
	return result, err
}

func selectedAPIKeyConfigFiles(baseURL *string, specified bool) ([]profilemodel.ExportedSecretFile, []string, error) {
	if !specified {
		return nil, nil, nil
	}
	if baseURL == nil {
		return nil, []string{".prodex-profile.toml"}, nil
	}
	content, err := toml.Marshal(map[string]string{"openai_compatible_base_url": *baseURL})
	if err != nil {
		return nil, nil, errors.New("failed to serialize profile local config")
	}
	return []profilemodel.ExportedSecretFile{{Path: ".prodex-profile.toml", Text: string(content)}}, nil, nil
}

func (catalog *Catalog) selectedOpenAITargetLocked(ctx context.Context, name string) (Report, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Report{}, errors.New("selected login profile is required")
	}
	listed, err := catalog.list(ctx)
	if err != nil {
		return Report{}, err
	}
	for _, report := range listed {
		if report.Profile.Name != name {
			continue
		}
		if report.Profile.Provider.Kind != profileentity.ProviderOpenAI {
			return Report{}, fmt.Errorf(
				"profile %q uses %s; godex login --profile currently supports OpenAI/Codex profiles only",
				name, report.Profile.Provider.Kind,
			)
		}
		return report, nil
	}
	return Report{}, fmt.Errorf(profileNotFoundFormat, name)
}
