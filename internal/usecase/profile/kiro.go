package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func (catalog *Catalog) importKiro(ctx context.Context, request profilemodel.BuiltinImportRequest) (profilemodel.BuiltinImportResult, error) {
	credential, secrets, listed, err := catalog.loadKiroImport(ctx)
	if err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	if existing, ok := findKiroIdentity(listed, credential.Provider); ok {
		return catalog.updateKiroProfile(ctx, request, credential, secrets, listed, existing)
	}
	return catalog.createKiroProfile(ctx, request, credential, secrets, listed)
}

func (catalog *Catalog) loadKiroImport(ctx context.Context) (profilemodel.BuiltinCredential, map[string]string, []Report, error) {
	if catalog.kiroImport == nil {
		return profilemodel.BuiltinCredential{}, nil, nil, errors.New("Kiro profile import support is not configured")
	}
	credential, err := catalog.kiroImport.Load(ctx)
	if err != nil {
		return profilemodel.BuiltinCredential{}, nil, nil, err
	}
	if credential.Provider.Kind != string(profileentity.ProviderKiro) {
		return profilemodel.BuiltinCredential{}, nil, nil, errors.New("Kiro source returned an incompatible provider")
	}
	secrets, err := builtinSecretMap(credential.SecretFiles, kiroCredentialFile)
	if err != nil {
		return profilemodel.BuiltinCredential{}, nil, nil, err
	}
	listed, err := catalog.List(ctx)
	return credential, secrets, listed, err
}

func (catalog *Catalog) updateKiroProfile(
	ctx context.Context,
	request profilemodel.BuiltinImportRequest,
	credential profilemodel.BuiltinCredential,
	secrets map[string]string,
	listed []Report,
	existing Report,
) (profilemodel.BuiltinImportResult, error) {
	requested := strings.TrimSpace(request.Name)
	if requested != "" {
		if err := profileentity.ValidateName(requested); err != nil {
			return profilemodel.BuiltinImportResult{}, err
		}
		if requested != existing.Profile.Name {
			return profilemodel.BuiltinImportResult{}, fmt.Errorf("Kiro identity is already imported as profile %q", existing.Profile.Name)
		}
	}
	activate := request.Activate || !hasActiveProfile(listed)
	provider := providerFromSnapshot(credential.Provider)
	if err := catalog.profiles.ReplaceProvider(ctx, existing.Profile.Name, credential.Email, provider, secrets, activate); err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	return profilemodel.BuiltinImportResult{
		Profile: existing.Profile.Name, Provider: "kiro", Updated: true,
		Active: activate || existing.Active, Warning: credential.Warning,
	}, nil
}

func (catalog *Catalog) createKiroProfile(
	ctx context.Context,
	request profilemodel.BuiltinImportRequest,
	credential profilemodel.BuiltinCredential,
	secrets map[string]string,
	listed []Report,
) (profilemodel.BuiltinImportResult, error) {
	name, err := catalog.kiroImportName(listed, request.Name, credential)
	if err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	profile := profileentity.Profile{
		Name: name, CodexHome: catalog.profiles.ManagedHome(name), Managed: true,
		Email: credential.Email, Provider: providerFromSnapshot(credential.Provider),
	}
	if err := profileentity.Validate(profile); err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	activate := request.Activate || !hasActiveProfile(listed)
	if err := catalog.profiles.ImportProvider(ctx, profile, secrets, activate); err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	return profilemodel.BuiltinImportResult{
		Profile: name, Provider: "kiro", Active: activate, Warning: credential.Warning,
	}, nil
}

func findKiroIdentity(listed []Report, provider profilemodel.ProviderSnapshot) (Report, bool) {
	authKey := optionalSnapshotValue(provider.AuthKey)
	profileARN := optionalSnapshotValue(provider.ProfileARN)
	profileName := optionalSnapshotValue(provider.ProfileName)
	for _, report := range listed {
		if report.Profile.Provider.Kind != profileentity.ProviderKiro {
			continue
		}
		stored := report.Profile.Provider
		if strings.TrimSpace(stored.AuthKey) == authKey && optionalIdentityEqual(stored.ProfileARN, profileARN) && optionalIdentityEqual(stored.ProfileName, profileName) {
			return report, true
		}
	}
	return Report{}, false
}

func optionalIdentityEqual(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" || right == "" {
		return left == right
	}
	return strings.EqualFold(left, right)
}

func (catalog *Catalog) kiroImportName(listed []Report, requested string, credential profilemodel.BuiltinCredential) (string, error) {
	if requested = strings.TrimSpace(requested); requested != "" {
		if err := profileentity.ValidateName(requested); err != nil {
			return "", err
		}
		for _, report := range listed {
			if report.Profile.Name == requested {
				return "", fmt.Errorf("profile %q already exists", requested)
			}
		}
		return requested, nil
	}
	base := "kiro"
	if email := strings.TrimSpace(credential.Email); email != "" {
		base = sanitizeProfileSlug("kiro-" + email)
	} else if upstreamName := optionalSnapshotValue(credential.Provider.ProfileName); upstreamName != "" {
		base = sanitizeProfileSlug("kiro-" + upstreamName)
	}
	return uniqueProfileName(base, "kiro", listed), nil
}
