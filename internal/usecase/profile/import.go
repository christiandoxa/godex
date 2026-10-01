package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

type importPlan struct {
	actions       []importAction
	resolvedNames map[string]string
	activeTarget  string
	keepActive    bool
}

type importAction struct {
	source      profilemodel.ExportedProfile
	target      Report
	identity    accountentity.Identity
	identityKey string
	create      bool
}

func (catalog *Catalog) Import(ctx context.Context, request profilemodel.ImportRequest) (profilemodel.ImportResult, error) {
	if strings.TrimSpace(request.Path) == "" {
		return profilemodel.ImportResult{}, errors.New("profile import path is required")
	}
	request.Path = cleanBundlePath(request.Path)
	content, err := catalog.profiles.ReadBundle(request.Path)
	if err != nil {
		return profilemodel.ImportResult{}, err
	}
	payload, encrypted, err := catalog.profiles.DecodeBundle(content, request.Password)
	clearBundleBytes(content)
	if err != nil {
		return profilemodel.ImportResult{}, err
	}
	plan, err := catalog.planImport(ctx, payload)
	if err != nil {
		return profilemodel.ImportResult{}, err
	}
	result, err := catalog.applyImport(ctx, plan)
	if err != nil {
		return profilemodel.ImportResult{}, err
	}
	result.Path = request.Path
	result.Encrypted = encrypted
	return result, nil
}

func (catalog *Catalog) planImport(ctx context.Context, payload profilemodel.BundlePayload) (importPlan, error) {
	listed, err := catalog.List(ctx)
	if err != nil {
		return importPlan{}, err
	}
	existingByName, identityTargets, keepActive := catalog.indexImportTargets(ctx, listed)
	plan := importPlan{resolvedNames: make(map[string]string, len(payload.Profiles)), keepActive: keepActive}
	for _, source := range payload.Profiles {
		action, err := catalog.planImportedProfile(ctx, source, existingByName, identityTargets)
		if err != nil {
			return importPlan{}, err
		}
		plan.actions = append(plan.actions, action)
		plan.resolvedNames[source.Name] = action.target.Profile.Name
		if action.identityKey != "" {
			identityTargets[action.identityKey] = action.target
		}
	}
	if payload.ActiveProfile != nil {
		resolved, ok := plan.resolvedNames[*payload.ActiveProfile]
		if !ok {
			return importPlan{}, fmt.Errorf("profile export active profile %q is missing", *payload.ActiveProfile)
		}
		if !plan.keepActive {
			plan.activeTarget = resolved
		}
	}
	return plan, nil
}

func (catalog *Catalog) indexImportTargets(ctx context.Context, listed []Report) (map[string]Report, map[string]Report, bool) {
	byName := make(map[string]Report, len(listed))
	byIdentity := make(map[string]Report)
	keepActive := false
	for _, report := range listed {
		byName[report.Profile.Name] = report
		keepActive = keepActive || report.Active
		key, err := catalog.existingImportIdentityKey(ctx, report)
		if err == nil && key != "" {
			byIdentity[key] = report
		}
	}
	return byName, byIdentity, keepActive
}

func (catalog *Catalog) planImportedProfile(
	ctx context.Context,
	source profilemodel.ExportedProfile,
	existingByName map[string]Report,
	identityTargets map[string]Report,
) (importAction, error) {
	if err := catalog.validateImportedProfile(ctx, source); err != nil {
		return importAction{}, err
	}
	identity, identityKey, err := catalog.importedIdentity(ctx, source)
	if err != nil {
		return importAction{}, err
	}
	if existing, ok := existingByName[source.Name]; ok {
		if err := catalog.validateNamedImportTarget(ctx, source, identityKey, existing); err != nil {
			return importAction{}, err
		}
		return importAction{source: source, target: existing, identity: identity, identityKey: identityKey}, nil
	}
	if existing, ok := identityTargets[identityKey]; identityKey != "" && ok {
		return importAction{source: source, target: existing, identity: identity, identityKey: identityKey}, nil
	}
	profile, err := catalog.newImportedProfile(source, identity)
	if err != nil {
		return importAction{}, err
	}
	return importAction{source: source, target: Report{Profile: profile}, identity: identity, identityKey: identityKey, create: true}, nil
}

func (catalog *Catalog) importedIdentity(ctx context.Context, source profilemodel.ExportedProfile) (accountentity.Identity, string, error) {
	if sourceProviderKind(source) != profileentity.ProviderOpenAI {
		return accountentity.Identity{}, "", nil
	}
	if catalog.auth == nil {
		return accountentity.Identity{}, "", errors.New("profile auth inspection is not configured")
	}
	authBytes := []byte(source.AuthJSON)
	defer clearBundleBytes(authBytes)
	identity, err := catalog.auth.InspectAuthJSON(ctx, authBytes)
	if err != nil {
		return accountentity.Identity{}, "", fmt.Errorf("profile %q has invalid OpenAI authentication", source.Name)
	}
	return identity, importIdentityKey(identity), nil
}

func (catalog *Catalog) validateNamedImportTarget(ctx context.Context, source profilemodel.ExportedProfile, key string, existing Report) error {
	sourceKind := sourceProviderKind(source)
	if existing.Profile.Provider.Kind != sourceKind {
		return fmt.Errorf(
			"profile %q already exists with provider %q and cannot be imported as %q",
			source.Name, existing.Profile.Provider.Kind, sourceKind,
		)
	}
	if sourceKind != profileentity.ProviderOpenAI {
		return nil
	}
	existingIdentity, err := catalog.existingIdentity(ctx, existing)
	if err != nil || key == "" || importIdentityKey(existingIdentity) != key {
		return fmt.Errorf("profile %q already exists with a different or unverifiable identity", source.Name)
	}
	return nil
}

func (catalog *Catalog) newImportedProfile(source profilemodel.ExportedProfile, identity accountentity.Identity) (profileentity.Profile, error) {
	kind := sourceProviderKind(source)
	profile := profileentity.Profile{
		Name: source.Name, CodexHome: catalog.profiles.ManagedHome(source.Name), Managed: true,
		Email: importedProfileEmail(source, identity), Provider: providerFromSnapshot(source.Provider),
	}
	profile.Provider.Kind = kind
	return profile, profileentity.Validate(profile)
}

func importedProfileEmail(source profilemodel.ExportedProfile, identity accountentity.Identity) string {
	if email := strings.TrimSpace(identity.Email); email != "" {
		return email
	}
	if source.Email != nil {
		return strings.TrimSpace(*source.Email)
	}
	return ""
}

func (catalog *Catalog) validateImportedProfile(ctx context.Context, source profilemodel.ExportedProfile) error {
	if err := profileentity.ValidateName(source.Name); err != nil {
		return err
	}
	switch sourceProviderKind(source) {
	case profileentity.ProviderOpenAI:
		if len(source.SecretFiles) != 0 {
			return fmt.Errorf("profile %q contains unexpected provider secret files", source.Name)
		}
		if strings.TrimSpace(source.AuthJSON) == "" {
			return fmt.Errorf("profile %q has no OpenAI authentication", source.Name)
		}
		return nil
	case profileentity.ProviderAnthropic:
		_, err := catalog.inspectAnthropicSecret(ctx, source)
		return err
	default:
		return fmt.Errorf("profile provider %q import is not implemented yet", sourceProviderKind(source))
	}
}

func (catalog *Catalog) existingImportIdentityKey(ctx context.Context, report Report) (string, error) {
	if !providerSupportsCodexRuntime(report.Profile.Provider.Kind) {
		return "", nil
	}
	identity, err := catalog.existingIdentity(ctx, report)
	if err != nil {
		return "", err
	}
	return importIdentityKey(identity), nil
}

func (catalog *Catalog) existingIdentity(ctx context.Context, report Report) (accountentity.Identity, error) {
	if catalog.auth == nil {
		return accountentity.Identity{}, errors.New("profile auth inspection is not configured")
	}
	authJSON, err := catalog.profiles.ReadAuthJSON(report.Profile.CodexHome)
	if err != nil {
		return accountentity.Identity{}, err
	}
	defer clearBundleBytes(authJSON)
	return catalog.auth.InspectAuthJSON(ctx, authJSON)
}

func importIdentityKey(identity accountentity.Identity) string {
	accountID := strings.TrimSpace(identity.ChatGPTAccountID)
	email := strings.ToLower(strings.TrimSpace(identity.Email))
	switch {
	case accountID != "" && email != "":
		return "account:" + accountID + "|email:" + email
	case accountID != "":
		return "account:" + accountID
	case email != "":
		return "email:" + email
	default:
		return ""
	}
}
