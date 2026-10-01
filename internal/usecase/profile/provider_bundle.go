package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func providerSnapshotFromEntity(provider profileentity.Provider) profilemodel.ProviderSnapshot {
	return profilemodel.ProviderSnapshot{
		Kind:          string(provider.Kind),
		ProjectID:     optionalString(provider.ProjectID),
		Account:       optionalString(provider.Account),
		AuthMethod:    optionalString(provider.AuthMethod),
		Host:          optionalString(provider.Host),
		Login:         optionalString(provider.Login),
		APIURL:        optionalString(provider.APIURL),
		AccessTypeSKU: optionalString(provider.AccessTypeSKU),
		CopilotPlan:   optionalString(provider.CopilotPlan),
		AuthKind:      optionalString(provider.AuthKind),
		ProfileName:   optionalString(provider.ProfileName),
		Region:        optionalString(provider.Region),
	}
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	copy := value
	return &copy
}

func sourceProviderKind(source profilemodel.ExportedProfile) profileentity.ProviderKind {
	kind := strings.TrimSpace(source.Provider.Kind)
	if kind == "" {
		return profileentity.ProviderOpenAI
	}
	return profileentity.ProviderKind(kind)
}

func providerSupportsCodexRuntime(kind profileentity.ProviderKind) bool {
	// Prodex 0.434.3 treats OpenAI as the native-Codex route. Anthropic is a
	// Responses adapter, so bundle identity de-duplication is intentionally not
	// applied across names for Anthropic profiles.
	return kind == profileentity.ProviderOpenAI
}

func (catalog *Catalog) inspectAnthropicSecret(ctx context.Context, source profilemodel.ExportedProfile) (profilemodel.BuiltinCredential, error) {
	if catalog.claude == nil {
		return profilemodel.BuiltinCredential{}, errors.New("Claude profile import support is not configured")
	}
	secret, err := requiredSecretFile(source.SecretFiles, claudeCredentialFile)
	if err != nil {
		return profilemodel.BuiltinCredential{}, fmt.Errorf("profile %q: %w", source.Name, err)
	}
	credential, err := catalog.claude.InspectCredential(ctx, secret.Text)
	if err != nil {
		return profilemodel.BuiltinCredential{}, fmt.Errorf("profile %q has invalid Claude credentials", source.Name)
	}
	if credential.Provider.Kind != string(profileentity.ProviderAnthropic) {
		return profilemodel.BuiltinCredential{}, fmt.Errorf("profile %q has incompatible Claude credentials", source.Name)
	}
	return credential, nil
}

func requiredSecretFile(files []profilemodel.ExportedSecretFile, required string) (profilemodel.ExportedSecretFile, error) {
	if len(files) != 1 {
		return profilemodel.ExportedSecretFile{}, fmt.Errorf("provider requires exactly one secret file %q", required)
	}
	file := files[0]
	if file.Path != required {
		return profilemodel.ExportedSecretFile{}, fmt.Errorf("unexpected provider secret file %q", file.Path)
	}
	if strings.TrimSpace(file.Text) == "" {
		return profilemodel.ExportedSecretFile{}, fmt.Errorf("provider secret file %q is empty", required)
	}
	return file, nil
}

func providerSecrets(source profilemodel.ExportedProfile) (map[string]string, error) {
	secrets := make(map[string]string, len(source.SecretFiles))
	for _, file := range source.SecretFiles {
		if file.Path == "" || file.Text == "" {
			return nil, errors.New("provider secret file is empty")
		}
		if _, exists := secrets[file.Path]; exists {
			return nil, fmt.Errorf("duplicate provider secret file %q", file.Path)
		}
		secrets[file.Path] = file.Text
	}
	return secrets, nil
}
