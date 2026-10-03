package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const (
	kiroCredentialFile   = "kiro_auth.json"
	kiroModelCatalogFile = "kiro_model_catalog.json"
	geminiOAuthFile      = "gemini_oauth.json"
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
		AuthKey:       optionalString(provider.AuthKey),
		AuthKind:      optionalString(provider.AuthKind),
		ProfileARN:    optionalString(provider.ProfileARN),
		ProfileName:   optionalString(provider.ProfileName),
		StartURL:      optionalString(provider.StartURL),
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
	// Prodex 0.435.0 treats OpenAI as the native-Codex route. Anthropic is a
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

func (catalog *Catalog) inspectKiroSecrets(ctx context.Context, source profilemodel.ExportedProfile) (profilemodel.BuiltinCredential, error) {
	if catalog.kiro == nil {
		return profilemodel.BuiltinCredential{}, errors.New("Kiro profile import support is not configured")
	}
	auth, catalogText, err := kiroSecretFiles(source.SecretFiles)
	if err != nil {
		return profilemodel.BuiltinCredential{}, fmt.Errorf("profile %q: %w", source.Name, err)
	}
	credential, err := catalog.kiro.InspectAuthSecret(ctx, auth.Text)
	if err != nil {
		return profilemodel.BuiltinCredential{}, fmt.Errorf("profile %q has invalid Kiro credentials", source.Name)
	}
	if credential.Provider.Kind != string(profileentity.ProviderKiro) {
		return profilemodel.BuiltinCredential{}, fmt.Errorf("profile %q has incompatible Kiro credentials", source.Name)
	}
	if catalogText != nil {
		if err := catalog.kiro.ValidateModelCatalog(ctx, catalogText.Text); err != nil {
			return profilemodel.BuiltinCredential{}, fmt.Errorf("profile %q has invalid Kiro model catalog", source.Name)
		}
	}
	return credential, nil
}

func inspectGeminiSecret(source profilemodel.ExportedProfile) (profilemodel.ExportedSecretFile, error) {
	if source.Provider.Kind != string(profileentity.ProviderGemini) || source.Provider.Email == nil {
		return profilemodel.ExportedSecretFile{}, fmt.Errorf("profile %q has invalid Gemini provider metadata", source.Name)
	}
	file, err := requiredSecretFile(source.SecretFiles, geminiOAuthFile)
	if err != nil {
		return profilemodel.ExportedSecretFile{}, fmt.Errorf("profile %q: %w", source.Name, err)
	}
	if !validGeminiOAuthJSON(file.Text) {
		return profilemodel.ExportedSecretFile{}, fmt.Errorf("profile %q has invalid Gemini OAuth credentials", source.Name)
	}
	return file, nil
}

func validGeminiOAuthJSON(text string) bool {
	decoder := json.NewDecoder(strings.NewReader(text))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := make(map[string]bool, 7)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		name, ok := token.(string)
		if !ok {
			return false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			clearBundleBytes(value)
			return false
		}
		value = bytes.TrimSpace(value)
		valid := true
		switch name {
		case "auth_mode", "access_token", "email":
			valid = !seen[name] && isJSONString(value)
			seen[name] = true
		case "refresh_token", "token_type", "scope", "project_id":
			valid = !seen[name] && (bytes.Equal(value, []byte("null")) || isJSONString(value))
			seen[name] = true
		case "expiry_date":
			var expiry *int64
			valid = !seen[name] && json.Unmarshal(value, &expiry) == nil
			seen[name] = true
		}
		clearBundleBytes(value)
		if !valid {
			return false
		}
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') || !seen["auth_mode"] || !seen["access_token"] || !seen["email"] {
		return false
	}
	var trailing json.RawMessage
	return decoder.Decode(&trailing) == io.EOF
}

func isJSONString(value []byte) bool {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' || !utf8.Valid(value) {
		return false
	}
	for index := 1; index < len(value)-1; index++ {
		if value[index] != '\\' {
			continue
		}
		if value[index+1] != 'u' {
			index++
			continue
		}
		if index+6 > len(value)-1 {
			return false
		}
		code, err := strconv.ParseUint(string(value[index+2:index+6]), 16, 16)
		if err != nil {
			return false
		}
		surrogate := uint16(code)
		switch {
		case surrogate >= 0xdc00 && surrogate <= 0xdfff:
			return false
		case surrogate >= 0xd800 && surrogate <= 0xdbff:
			if index+12 >= len(value) || value[index+6] != '\\' || value[index+7] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(value[index+8:index+12]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			index += 11
		default:
			index += 5
		}
	}
	return true
}

func kiroSecretFiles(files []profilemodel.ExportedSecretFile) (profilemodel.ExportedSecretFile, *profilemodel.ExportedSecretFile, error) {
	var auth profilemodel.ExportedSecretFile
	var modelCatalog *profilemodel.ExportedSecretFile
	seen := make(map[string]bool, len(files))
	for index := range files {
		file := files[index]
		if seen[file.Path] {
			return profilemodel.ExportedSecretFile{}, nil, fmt.Errorf("duplicate provider secret file %q", file.Path)
		}
		seen[file.Path] = true
		if strings.TrimSpace(file.Text) == "" {
			return profilemodel.ExportedSecretFile{}, nil, fmt.Errorf("provider secret file %q is empty", file.Path)
		}
		switch file.Path {
		case kiroCredentialFile:
			auth = file
		case kiroModelCatalogFile:
			copy := file
			modelCatalog = &copy
		default:
			return profilemodel.ExportedSecretFile{}, nil, fmt.Errorf("unexpected provider secret file %q", file.Path)
		}
	}
	if auth.Path == "" {
		return profilemodel.ExportedSecretFile{}, nil, fmt.Errorf("provider requires secret file %q", kiroCredentialFile)
	}
	return auth, modelCatalog, nil
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
