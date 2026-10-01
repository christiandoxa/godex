package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const copilotProviderLabel = "copilot"

func (catalog *Catalog) importCopilot(ctx context.Context, request profilemodel.BuiltinImportRequest) (profilemodel.BuiltinImportResult, error) {
	credential, listed, err := catalog.loadCopilotImport(ctx)
	if err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	if existing, ok := findCopilotIdentity(listed, credential.Provider, credential.Email); ok {
		return catalog.updateCopilotProfile(ctx, request, credential, listed, existing)
	}
	return catalog.createCopilotProfile(ctx, request, credential, listed)
}

func (catalog *Catalog) loadCopilotImport(ctx context.Context) (profilemodel.BuiltinCredential, []Report, error) {
	if catalog.copilot == nil {
		return profilemodel.BuiltinCredential{}, nil, errors.New("Copilot profile import support is not configured")
	}
	credential, err := catalog.copilot.Load(ctx)
	if err != nil {
		return profilemodel.BuiltinCredential{}, nil, err
	}
	if credential.Provider.Kind != string(profileentity.ProviderCopilot) {
		return profilemodel.BuiltinCredential{}, nil, errors.New("Copilot source returned an incompatible provider")
	}
	if strings.TrimSpace(optionalSnapshotValue(credential.Provider.Host)) == "" || strings.TrimSpace(optionalSnapshotValue(credential.Provider.Login)) == "" {
		return profilemodel.BuiltinCredential{}, nil, errors.New("Copilot source did not provide host/login identity")
	}
	listed, err := catalog.List(ctx)
	return credential, listed, err
}

func (catalog *Catalog) updateCopilotProfile(
	ctx context.Context,
	request profilemodel.BuiltinImportRequest,
	credential profilemodel.BuiltinCredential,
	listed []Report,
	existing Report,
) (profilemodel.BuiltinImportResult, error) {
	requested := strings.TrimSpace(request.Name)
	if requested != "" {
		if err := profileentity.ValidateName(requested); err != nil {
			return profilemodel.BuiltinImportResult{}, err
		}
		if requested != existing.Profile.Name {
			return profilemodel.BuiltinImportResult{}, fmt.Errorf("Copilot account %q is already imported as profile %q", credential.Email, existing.Profile.Name)
		}
	}
	activate := request.Activate || !hasActiveProfile(listed)
	provider := providerFromSnapshot(credential.Provider)
	if err := catalog.profiles.ReplaceProvider(ctx, existing.Profile.Name, credential.Email, provider, map[string]string{}, activate); err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	return profilemodel.BuiltinImportResult{
		Profile: existing.Profile.Name, Provider: copilotProviderLabel, Updated: true,
		Active: activate || existing.Active, Warning: credential.Warning,
	}, nil
}

func (catalog *Catalog) createCopilotProfile(
	ctx context.Context,
	request profilemodel.BuiltinImportRequest,
	credential profilemodel.BuiltinCredential,
	listed []Report,
) (profilemodel.BuiltinImportResult, error) {
	name, err := catalog.copilotImportName(listed, request.Name, credential.Email)
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
	if err := catalog.profiles.ImportProvider(ctx, profile, map[string]string{}, activate); err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	return profilemodel.BuiltinImportResult{
		Profile: name, Provider: copilotProviderLabel, Active: activate, Warning: credential.Warning,
	}, nil
}

func findCopilotIdentity(listed []Report, provider profilemodel.ProviderSnapshot, configLogin string) (Report, bool) {
	host := optionalSnapshotValue(provider.Host)
	login := strings.TrimSpace(configLogin)
	for _, report := range listed {
		if report.Profile.Provider.Kind != profileentity.ProviderCopilot {
			continue
		}
		if trimmedEqual(report.Profile.Provider.Host, host) && trimmedEqual(report.Profile.Provider.Login, login) {
			return report, true
		}
	}
	return Report{}, false
}

func trimmedEqual(left, right string) bool {
	return strings.TrimSpace(left) == strings.TrimSpace(right)
}

func (catalog *Catalog) copilotImportName(listed []Report, requested, login string) (string, error) {
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
	base := sanitizeProfileSlug("copilot-" + strings.TrimSpace(login))
	return uniqueProfileName(base, copilotProviderLabel, listed), nil
}
