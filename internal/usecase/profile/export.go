package profile

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	"github.com/christiandoxa/godex/internal/version"
)

func (catalog *Catalog) Export(ctx context.Context, request profilemodel.ExportRequest) (profilemodel.ExportResult, error) {
	if strings.TrimSpace(request.OutputPath) == "" {
		return profilemodel.ExportResult{}, errors.New("profile export output path is required")
	}
	request.OutputPath = cleanBundlePath(request.OutputPath)
	listed, err := catalog.List(ctx)
	if err != nil {
		return profilemodel.ExportResult{}, err
	}
	selected, err := selectExportProfiles(listed, request.Profiles)
	if err != nil {
		return profilemodel.ExportResult{}, err
	}
	payload, err := catalog.buildExportPayload(ctx, selected)
	if err != nil {
		return profilemodel.ExportResult{}, err
	}
	content, err := catalog.profiles.EncodeBundle(payload, request.Password)
	if err != nil {
		return profilemodel.ExportResult{}, err
	}
	if err := catalog.profiles.WriteBundle(request.OutputPath, content); err != nil {
		return profilemodel.ExportResult{}, err
	}
	return exportResult(payload, request.OutputPath, request.Password != ""), nil
}

func (catalog *Catalog) buildExportPayload(ctx context.Context, selected []Report) (profilemodel.BundlePayload, error) {
	payload := profilemodel.BundlePayload{
		ExportedAt:          time.Now().UTC().Format(time.RFC3339),
		SourceProdexVersion: "godex-" + version.Version,
		Profiles:            make([]profilemodel.ExportedProfile, 0, len(selected)),
	}
	for _, report := range selected {
		exported, err := catalog.exportProfile(ctx, report)
		if err != nil {
			return profilemodel.BundlePayload{}, err
		}
		payload.Profiles = append(payload.Profiles, exported)
		if report.Active {
			name := report.Profile.Name
			payload.ActiveProfile = &name
		}
	}
	return payload, nil
}

func (catalog *Catalog) exportProfile(ctx context.Context, report Report) (profilemodel.ExportedProfile, error) {
	switch report.Profile.Provider.Kind {
	case profileentity.ProviderOpenAI:
		return catalog.exportOpenAIProfile(ctx, report)
	case profileentity.ProviderGemini:
		return catalog.exportGeminiProfile(report)
	case profileentity.ProviderAnthropic:
		return catalog.exportAnthropicProfile(ctx, report)
	case profileentity.ProviderKiro:
		return catalog.exportKiroProfile(ctx, report)
	case profileentity.ProviderCopilot:
		return exportMetadataProfile(report), nil
	default:
		return profilemodel.ExportedProfile{}, fmt.Errorf("profile provider %q export is not implemented yet", report.Profile.Provider.Kind)
	}
}

func (catalog *Catalog) exportGeminiProfile(report Report) (profilemodel.ExportedProfile, error) {
	secret, err := catalog.profiles.ReadProviderSecret(report.Profile.CodexHome, geminiOAuthFile)
	if err != nil {
		return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: %w", report.Profile.Name, err)
	}
	email := report.Profile.Email
	exported := profilemodel.ExportedProfile{
		Name: report.Profile.Name, Email: optionalString(strings.TrimSpace(report.Profile.Email)),
		SourceManaged: report.Profile.Managed,
		Provider: profilemodel.ProviderSnapshot{
			Kind: string(profileentity.ProviderGemini), Email: &email,
			ProjectID: optionalString(report.Profile.Provider.ProjectID),
		},
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: secret}},
	}
	if _, err := inspectGeminiSecret(exported); err != nil {
		return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: Gemini OAuth credentials are invalid", report.Profile.Name)
	}
	return exported, nil
}

func (catalog *Catalog) exportOpenAIProfile(ctx context.Context, report Report) (profilemodel.ExportedProfile, error) {
	if catalog.auth == nil {
		return profilemodel.ExportedProfile{}, errors.New("profile auth inspection is not configured")
	}
	authJSON, err := catalog.profiles.ReadAuthJSON(report.Profile.CodexHome)
	if err != nil {
		return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: %w", report.Profile.Name, err)
	}
	defer clearBundleBytes(authJSON)
	identity, err := catalog.auth.InspectAuthJSON(ctx, authJSON)
	if err != nil {
		return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: authentication is not a usable ChatGPT profile", report.Profile.Name)
	}
	email := strings.TrimSpace(identity.Email)
	if email == "" {
		email = strings.TrimSpace(report.Profile.Email)
	}
	return profilemodel.ExportedProfile{
		Name: report.Profile.Name, Email: optionalString(email), SourceManaged: report.Profile.Managed,
		Provider: profilemodel.ProviderSnapshot{Kind: string(profileentity.ProviderOpenAI)},
		AuthJSON: string(authJSON), SecretFiles: []profilemodel.ExportedSecretFile{},
	}, nil
}

func (catalog *Catalog) exportAnthropicProfile(ctx context.Context, report Report) (profilemodel.ExportedProfile, error) {
	secret, err := catalog.profiles.ReadProviderSecret(report.Profile.CodexHome, claudeCredentialFile)
	if err != nil {
		return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: %w", report.Profile.Name, err)
	}
	if catalog.claude == nil {
		return profilemodel.ExportedProfile{}, errors.New("Claude profile export support is not configured")
	}
	if _, err := catalog.claude.InspectCredential(ctx, secret); err != nil {
		return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: Claude credentials are invalid", report.Profile.Name)
	}
	return profilemodel.ExportedProfile{
		Name: report.Profile.Name, Email: optionalString(strings.TrimSpace(report.Profile.Email)),
		SourceManaged: report.Profile.Managed, Provider: providerSnapshotFromEntity(report.Profile.Provider),
		AuthJSON: "", SecretFiles: []profilemodel.ExportedSecretFile{{Path: claudeCredentialFile, Text: secret}},
	}, nil
}

func (catalog *Catalog) exportKiroProfile(ctx context.Context, report Report) (profilemodel.ExportedProfile, error) {
	if catalog.kiro == nil {
		return profilemodel.ExportedProfile{}, errors.New("Kiro profile export support is not configured")
	}
	auth, err := catalog.profiles.ReadProviderSecret(report.Profile.CodexHome, kiroCredentialFile)
	if err != nil {
		return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: %w", report.Profile.Name, err)
	}
	if _, err := catalog.kiro.InspectAuthSecret(ctx, auth); err != nil {
		return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: Kiro credentials are invalid", report.Profile.Name)
	}
	secrets := []profilemodel.ExportedSecretFile{{Path: kiroCredentialFile, Text: auth}}
	modelCatalog, found, err := catalog.profiles.ReadOptionalProviderSecret(report.Profile.CodexHome, kiroModelCatalogFile)
	if err != nil {
		return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: %w", report.Profile.Name, err)
	}
	if found {
		if err := catalog.kiro.ValidateModelCatalog(ctx, modelCatalog); err != nil {
			return profilemodel.ExportedProfile{}, fmt.Errorf("export profile %q: Kiro model catalog is invalid", report.Profile.Name)
		}
		secrets = append(secrets, profilemodel.ExportedSecretFile{Path: kiroModelCatalogFile, Text: modelCatalog})
	}
	return profilemodel.ExportedProfile{
		Name: report.Profile.Name, Email: optionalString(strings.TrimSpace(report.Profile.Email)),
		SourceManaged: report.Profile.Managed, Provider: providerSnapshotFromEntity(report.Profile.Provider),
		AuthJSON: "", SecretFiles: secrets,
	}, nil
}

func exportMetadataProfile(report Report) profilemodel.ExportedProfile {
	return profilemodel.ExportedProfile{
		Name: report.Profile.Name, Email: optionalString(strings.TrimSpace(report.Profile.Email)),
		SourceManaged: report.Profile.Managed, Provider: providerSnapshotFromEntity(report.Profile.Provider),
		AuthJSON: "", SecretFiles: []profilemodel.ExportedSecretFile{},
	}
}

func selectExportProfiles(listed []Report, requested []string) ([]Report, error) {
	if len(listed) == 0 {
		return nil, errors.New("no profiles configured")
	}
	byName := make(map[string]Report, len(listed))
	for _, report := range listed {
		byName[report.Profile.Name] = report
	}
	if len(requested) == 0 {
		selected := append([]Report(nil), listed...)
		sort.Slice(selected, func(i, j int) bool { return selected[i].Profile.Name < selected[j].Profile.Name })
		return selected, nil
	}
	selected := make([]Report, 0, len(requested))
	seen := make(map[string]bool, len(requested))
	for _, name := range requested {
		if seen[name] {
			continue
		}
		seen[name] = true
		report, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf(profileNotFoundFormat, name)
		}
		selected = append(selected, report)
	}
	return selected, nil
}

func exportResult(payload profilemodel.BundlePayload, path string, encrypted bool) profilemodel.ExportResult {
	result := profilemodel.ExportResult{ProfileCount: len(payload.Profiles), Path: path, Encrypted: encrypted}
	if payload.ActiveProfile != nil {
		result.ActiveProfile = *payload.ActiveProfile
	}
	return result
}
