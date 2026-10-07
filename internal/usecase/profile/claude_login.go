package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

type claudeOAuthLoginSource interface {
	LoginOAuth(context.Context, string, string) (profilemodel.BuiltinCredential, error)
}

type claudeLoginStagingStore interface {
	CreateStagedHome() (string, error)
	RemoveStagedHome(string) error
}

func (catalog *Catalog) LoginClaude(
	ctx context.Context,
	selectedName string,
	requestedName string,
) (profilemodel.BuiltinImportResult, error) {
	source, ok := catalog.claude.(claudeOAuthLoginSource)
	if !ok {
		return profilemodel.BuiltinImportResult{}, errors.New("Claude OAuth login support is not configured")
	}
	stager, ok := catalog.accounts.(claudeLoginStagingStore)
	if !ok {
		return profilemodel.BuiltinImportResult{}, errors.New("Claude OAuth login staging is not configured")
	}
	selectedName = strings.TrimSpace(selectedName)
	requestedName = strings.TrimSpace(requestedName)

	if selectedName != "" {
		var result profilemodel.BuiltinImportResult
		err := catalog.withBundleImportLock(ctx, func() (runErr error) {
			target, err := catalog.selectedClaudeTargetLocked(ctx, selectedName)
			if err != nil {
				return err
			}
			staged, err := stager.CreateStagedHome()
			if err != nil {
				return err
			}
			defer func() { runErr = errors.Join(runErr, stager.RemoveStagedHome(staged)) }()

			credential, err := source.LoginOAuth(ctx, staged, "")
			if err != nil {
				return err
			}
			current, err := catalog.selectedClaudeTargetLocked(ctx, selectedName)
			if err != nil || current.AccountID != target.AccountID || current.Profile != target.Profile {
				return fmt.Errorf("profile %q changed while login was running", selectedName)
			}
			result, err = catalog.commitClaudeCredentialLocked(ctx, credential, &target, "")
			return err
		})
		return result, err
	}

	staged, err := stager.CreateStagedHome()
	if err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	credential, loginErr := source.LoginOAuth(ctx, staged, "")
	cleanupErr := stager.RemoveStagedHome(staged)
	if loginErr != nil || cleanupErr != nil {
		return profilemodel.BuiltinImportResult{}, errors.Join(loginErr, cleanupErr)
	}

	var result profilemodel.BuiltinImportResult
	err = catalog.withBundleImportLock(ctx, func() error {
		var commitErr error
		result, commitErr = catalog.commitClaudeCredentialLocked(ctx, credential, nil, requestedName)
		return commitErr
	})
	return result, err
}

func (catalog *Catalog) selectedClaudeTargetLocked(ctx context.Context, name string) (Report, error) {
	listed, err := catalog.list(ctx)
	if err != nil {
		return Report{}, err
	}
	for _, report := range listed {
		if report.Profile.Name != name {
			continue
		}
		switch report.Profile.Provider.Kind {
		case profileentity.ProviderGemini, profileentity.ProviderCopilot, profileentity.ProviderAgy,
			profileentity.ProviderDeepSeek, profileentity.ProviderLocal:
			return Report{}, fmt.Errorf(
				"profile %q uses %s; Claude sign-in supports OpenAI/Codex placeholders or Anthropic Claude profiles",
				name, report.Profile.Provider.Kind,
			)
		default:
			return report, nil
		}
	}
	return Report{}, fmt.Errorf(profileNotFoundFormat, name)
}

func (catalog *Catalog) commitClaudeCredentialLocked(
	ctx context.Context,
	credential profilemodel.BuiltinCredential,
	selected *Report,
	requestedName string,
) (profilemodel.BuiltinImportResult, error) {
	if credential.Provider.Kind != string(profileentity.ProviderAnthropic) {
		return profilemodel.BuiltinImportResult{}, errors.New("Claude OAuth login returned an incompatible provider")
	}
	if _, err := builtinSecretMap(credential.SecretFiles, claudeCredentialFile); err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	listed, err := catalog.list(ctx)
	if err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}

	var target Report
	updated := false
	switch {
	case selected != nil:
		target = *selected
		updated = true
	case requestedName == "":
		if existing, ok := findAnthropicIdentity(listed, credential.Provider); ok {
			target = existing
			updated = true
		}
	}
	if !updated {
		name, err := catalog.claudeImportName(listed, requestedName, credential.Email)
		if err != nil {
			return profilemodel.BuiltinImportResult{}, err
		}
		target = Report{Profile: profileentity.Profile{
			Name: name, CodexHome: catalog.profiles.ManagedHome(name), Managed: true,
			Email: credential.Email, Provider: providerFromSnapshot(credential.Provider),
		}}
	}

	desired := target.Profile
	desired.Email = credential.Email
	desired.Provider = providerFromSnapshot(credential.Provider)
	source := profilemodel.ExportedProfile{
		Name: desired.Name, Email: optionalStringPointer(credential.Email), SourceManaged: desired.Managed,
		Provider: credential.Provider, SecretFiles: append([]profilemodel.ExportedSecretFile(nil), credential.SecretFiles...),
	}
	action := importAction{source: source, target: target, create: !updated}
	if updated {
		action.after = &desired
	}
	plan := importPlan{
		actions: []importAction{action}, resolvedNames: map[string]string{desired.Name: desired.Name},
		activeTarget: desired.Name,
	}
	if _, err := catalog.applyImport(ctx, plan); err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	return profilemodel.BuiltinImportResult{
		Profile: desired.Name, Provider: "anthropic", Updated: updated, Active: true,
	}, nil
}

func optionalStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
