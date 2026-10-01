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
	source   profilemodel.ExportedProfile
	target   Report
	identity accountentity.Identity
	create   bool
}

func (catalog *Catalog) Import(ctx context.Context, request profilemodel.ImportRequest) (profilemodel.ImportResult, error) {
	if strings.TrimSpace(request.Path) == "" {
		return profilemodel.ImportResult{}, errors.New("profile import path is required")
	}
	request.Path = cleanBundlePath(request.Path)
	if catalog.auth == nil {
		return profilemodel.ImportResult{}, errors.New("profile auth inspection is not configured")
	}
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
		if key := importIdentityKey(action.identity); key != "" {
			identityTargets[key] = action.target
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
		identity, err := catalog.existingIdentity(ctx, report)
		if err != nil {
			continue
		}
		if key := importIdentityKey(identity); key != "" {
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
	if err := validateImportedProfile(source); err != nil {
		return importAction{}, err
	}
	identity, err := catalog.importedIdentity(ctx, source)
	if err != nil {
		return importAction{}, err
	}
	key := importIdentityKey(identity)
	if existing, ok := existingByName[source.Name]; ok {
		if err := catalog.validateNamedImportTarget(ctx, source.Name, key, existing); err != nil {
			return importAction{}, err
		}
		return importAction{source: source, target: existing, identity: identity}, nil
	}
	if existing, ok := identityTargets[key]; key != "" && ok {
		return importAction{source: source, target: existing, identity: identity}, nil
	}
	profile, err := catalog.newImportedProfile(source, identity)
	if err != nil {
		return importAction{}, err
	}
	return importAction{source: source, target: Report{Profile: profile}, identity: identity, create: true}, nil
}

func (catalog *Catalog) importedIdentity(ctx context.Context, source profilemodel.ExportedProfile) (accountentity.Identity, error) {
	authBytes := []byte(source.AuthJSON)
	defer clearBundleBytes(authBytes)
	identity, err := catalog.auth.InspectAuthJSON(ctx, authBytes)
	if err != nil {
		return accountentity.Identity{}, fmt.Errorf("profile %q has invalid OpenAI authentication", source.Name)
	}
	return identity, nil
}

func (catalog *Catalog) validateNamedImportTarget(ctx context.Context, sourceName, key string, existing Report) error {
	if existing.Profile.Provider.Kind != profileentity.ProviderOpenAI {
		return fmt.Errorf("profile %q already exists with an incompatible provider", sourceName)
	}
	existingIdentity, err := catalog.existingIdentity(ctx, existing)
	if err != nil || key == "" || importIdentityKey(existingIdentity) != key {
		return fmt.Errorf("profile %q already exists with a different or unverifiable identity", sourceName)
	}
	return nil
}

func (catalog *Catalog) newImportedProfile(source profilemodel.ExportedProfile, identity accountentity.Identity) (profileentity.Profile, error) {
	email := strings.TrimSpace(identity.Email)
	if email == "" && source.Email != nil {
		email = strings.TrimSpace(*source.Email)
	}
	profile := profileentity.Profile{
		Name: source.Name, CodexHome: catalog.profiles.ManagedHome(source.Name), Managed: true, Email: email,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	return profile, profileentity.Validate(profile)
}

func validateImportedProfile(source profilemodel.ExportedProfile) error {
	if err := profileentity.ValidateName(source.Name); err != nil {
		return err
	}
	provider := source.Provider.Kind
	if provider == "" {
		provider = string(profileentity.ProviderOpenAI)
	}
	if provider != string(profileentity.ProviderOpenAI) {
		return fmt.Errorf("profile provider %q import is not implemented yet", provider)
	}
	if len(source.SecretFiles) != 0 {
		return fmt.Errorf("profile %q contains provider secret files that are not supported yet", source.Name)
	}
	if strings.TrimSpace(source.AuthJSON) == "" {
		return fmt.Errorf("profile %q has no OpenAI authentication", source.Name)
	}
	return nil
}

func (catalog *Catalog) existingIdentity(ctx context.Context, report Report) (accountentity.Identity, error) {
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
