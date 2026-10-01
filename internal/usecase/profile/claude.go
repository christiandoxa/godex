package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const claudeCredentialFile = ".credentials.json"

func (catalog *Catalog) ImportBuiltin(ctx context.Context, request profilemodel.BuiltinImportRequest) (profilemodel.BuiltinImportResult, error) {
	switch strings.ToLower(strings.TrimSpace(request.Source)) {
	case "claude":
		return catalog.importClaude(ctx, request)
	default:
		return profilemodel.BuiltinImportResult{}, fmt.Errorf("unsupported built-in profile source %q", request.Source)
	}
}

func (catalog *Catalog) importClaude(ctx context.Context, request profilemodel.BuiltinImportRequest) (profilemodel.BuiltinImportResult, error) {
	credential, secrets, listed, err := catalog.loadClaudeImport(ctx)
	if err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	if strings.TrimSpace(request.Name) == "" {
		if existing, ok := findAnthropicIdentity(listed, credential.Provider); ok {
			return catalog.updateClaudeProfile(ctx, request, credential, secrets, existing)
		}
	}
	return catalog.createClaudeProfile(ctx, request, credential, secrets, listed)
}

func (catalog *Catalog) loadClaudeImport(ctx context.Context) (profilemodel.BuiltinCredential, map[string]string, []Report, error) {
	if catalog.claude == nil {
		return profilemodel.BuiltinCredential{}, nil, nil, errors.New("Claude profile import support is not configured")
	}
	credential, err := catalog.claude.Load(ctx)
	if err != nil {
		return profilemodel.BuiltinCredential{}, nil, nil, err
	}
	if credential.Provider.Kind != string(profileentity.ProviderAnthropic) {
		return profilemodel.BuiltinCredential{}, nil, nil, errors.New("Claude source returned an incompatible provider")
	}
	secrets, err := builtinSecretMap(credential.SecretFiles, claudeCredentialFile)
	if err != nil {
		return profilemodel.BuiltinCredential{}, nil, nil, err
	}
	listed, err := catalog.List(ctx)
	return credential, secrets, listed, err
}

func (catalog *Catalog) updateClaudeProfile(
	ctx context.Context,
	request profilemodel.BuiltinImportRequest,
	credential profilemodel.BuiltinCredential,
	secrets map[string]string,
	existing Report,
) (profilemodel.BuiltinImportResult, error) {
	provider := providerFromSnapshot(credential.Provider)
	if err := catalog.profiles.ReplaceProvider(ctx, existing.Profile.Name, credential.Email, provider, secrets, request.Activate); err != nil {
		return profilemodel.BuiltinImportResult{}, err
	}
	return profilemodel.BuiltinImportResult{
		Profile: existing.Profile.Name, Provider: "anthropic", Updated: true, Active: request.Activate || existing.Active,
	}, nil
}

func (catalog *Catalog) createClaudeProfile(
	ctx context.Context,
	request profilemodel.BuiltinImportRequest,
	credential profilemodel.BuiltinCredential,
	secrets map[string]string,
	listed []Report,
) (profilemodel.BuiltinImportResult, error) {
	name, err := catalog.claudeImportName(listed, request.Name, credential.Email)
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
	return profilemodel.BuiltinImportResult{Profile: name, Provider: "anthropic", Active: activate}, nil
}

func builtinSecretMap(files []profilemodel.ExportedSecretFile, required string) (map[string]string, error) {
	secrets := make(map[string]string, len(files))
	for _, file := range files {
		if file.Path == "" || file.Text == "" {
			return nil, errors.New("built-in provider secret file is empty")
		}
		if _, exists := secrets[file.Path]; exists {
			return nil, fmt.Errorf("duplicate provider secret file %q", file.Path)
		}
		secrets[file.Path] = file.Text
	}
	if _, ok := secrets[required]; !ok {
		return nil, fmt.Errorf("built-in provider source is missing %q", required)
	}
	return secrets, nil
}

func findAnthropicIdentity(listed []Report, provider profilemodel.ProviderSnapshot) (Report, bool) {
	account := optionalSnapshotValue(provider.Account)
	authMethod := optionalSnapshotValue(provider.AuthMethod)
	for _, report := range listed {
		if report.Profile.Provider.Kind != profileentity.ProviderAnthropic {
			continue
		}
		if normalizedEqual(report.Profile.Provider.Account, account) && normalizedEqual(report.Profile.Provider.AuthMethod, authMethod) {
			return report, true
		}
	}
	return Report{}, false
}

func (catalog *Catalog) claudeImportName(listed []Report, requested, account string) (string, error) {
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
	base := sanitizeProfileSlug("claude-" + account)
	if strings.TrimSpace(account) == "" {
		base = "claude"
	}
	return uniqueProfileName(base, "claude", listed), nil
}

func providerFromSnapshot(snapshot profilemodel.ProviderSnapshot) profileentity.Provider {
	return profileentity.Provider{
		Kind:       profileentity.ProviderKind(snapshot.Kind),
		Account:    optionalSnapshotValue(snapshot.Account),
		AuthMethod: optionalSnapshotValue(snapshot.AuthMethod),
		ProjectID:  optionalSnapshotValue(snapshot.ProjectID),
		Host:       optionalSnapshotValue(snapshot.Host),
		Login:      optionalSnapshotValue(snapshot.Login),
		APIURL:     optionalSnapshotValue(snapshot.APIURL),
	}
}

func optionalSnapshotValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func normalizedEqual(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func hasActiveProfile(listed []Report) bool {
	for _, report := range listed {
		if report.Active {
			return true
		}
	}
	return false
}

func uniqueProfileName(base, fallback string, listed []Report) string {
	if strings.TrimSpace(base) == "" {
		base = fallback
	}
	used := make(map[string]bool, len(listed))
	for _, report := range listed {
		used[report.Profile.Name] = true
	}
	if !used[base] {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if !used[candidate] {
			return candidate
		}
	}
}

func sanitizeProfileSlug(value string) string {
	value = strings.TrimSpace(value)
	var output strings.Builder
	for len(value) > 0 {
		current, width := utf8.DecodeRuneInString(value)
		value = value[width:]
		if current >= 'A' && current <= 'Z' {
			current += 'a' - 'A'
		}
		switch {
		case current >= 'a' && current <= 'z', current >= '0' && current <= '9', current == '.', current == '_', current == '-':
			output.WriteRune(current)
		case current == '@':
			output.WriteByte('_')
		default:
			output.WriteByte('-')
		}
	}
	result := strings.Trim(output.String(), "._-")
	if result == "" {
		return "profile"
	}
	return result
}
