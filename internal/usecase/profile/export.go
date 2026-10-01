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
	if catalog.auth == nil {
		return profilemodel.ExportResult{}, errors.New("profile auth inspection is not configured")
	}
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
	if report.Profile.Provider.Kind != profileentity.ProviderOpenAI {
		return profilemodel.ExportedProfile{}, fmt.Errorf("profile provider %q export is not implemented yet", report.Profile.Provider.Kind)
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
	var emailPointer *string
	if email != "" {
		emailPointer = &email
	}
	return profilemodel.ExportedProfile{
		Name: report.Profile.Name, Email: emailPointer, SourceManaged: report.Profile.Managed,
		Provider: profilemodel.ProviderSnapshot{Kind: string(profileentity.ProviderOpenAI)},
		AuthJSON: string(authJSON), SecretFiles: []profilemodel.ExportedSecretFile{},
	}, nil
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
