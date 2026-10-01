package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const (
	CredentialsFile  = "kiro_auth.json"
	ModelCatalogFile = "kiro_model_catalog.json"
	catalogHardLimit = 1024
	secretMaxBytes   = 2 << 20
)

type Inspector struct{}

type authSecret struct {
	AuthKey     string  `json:"auth_key"`
	AuthKind    string  `json:"auth_kind"`
	AuthJSON    string  `json:"auth_json"`
	Email       *string `json:"email"`
	ProfileARN  *string `json:"profile_arn"`
	ProfileName *string `json:"profile_name"`
	StartURL    *string `json:"start_url"`
	Region      *string `json:"region"`
}

func NewInspector() *Inspector { return &Inspector{} }

func (inspector *Inspector) InspectAuthSecret(ctx context.Context, text string) (profilemodel.BuiltinCredential, error) {
	if err := ctx.Err(); err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	if len(text) == 0 || len(text) > secretMaxBytes {
		return profilemodel.BuiltinCredential{}, errors.New("Kiro auth secret is empty or exceeds the safe size limit")
	}
	var secret authSecret
	if err := json.Unmarshal([]byte(text), &secret); err != nil {
		return profilemodel.BuiltinCredential{}, errors.New("failed to parse Kiro auth secret JSON")
	}
	if strings.TrimSpace(secret.AuthKey) == "" {
		return profilemodel.BuiltinCredential{}, errors.New("Kiro auth secret is missing auth_key")
	}
	if strings.TrimSpace(secret.AuthKind) == "" {
		return profilemodel.BuiltinCredential{}, errors.New("Kiro auth secret is missing auth_kind")
	}
	if strings.TrimSpace(secret.AuthJSON) == "" {
		return profilemodel.BuiltinCredential{}, errors.New("Kiro auth secret is missing auth_json")
	}
	var embedded any
	if err := json.Unmarshal([]byte(secret.AuthJSON), &embedded); err != nil {
		return profilemodel.BuiltinCredential{}, errors.New("failed to parse embedded Kiro auth_json")
	}
	provider := profilemodel.ProviderSnapshot{
		Kind:        "kiro",
		AuthKey:     optional(strings.TrimSpace(secret.AuthKey)),
		AuthKind:    optional(strings.TrimSpace(secret.AuthKind)),
		ProfileARN:  normalizedOptional(secret.ProfileARN),
		ProfileName: normalizedOptional(secret.ProfileName),
		StartURL:    normalizedOptional(secret.StartURL),
		Region:      normalizedOptional(secret.Region),
	}
	email := ""
	if secret.Email != nil {
		email = strings.TrimSpace(*secret.Email)
	}
	return profilemodel.BuiltinCredential{
		Provider: provider,
		Email:    email,
		SecretFiles: []profilemodel.ExportedSecretFile{{
			Path: CredentialsFile,
			Text: text,
		}},
	}, nil
}

func (inspector *Inspector) ValidateModelCatalog(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := normalizeModelCatalogText(text)
	return err
}

func normalizeModelCatalogText(text string) (string, error) {
	if len(text) == 0 || len(text) > secretMaxBytes {
		return "", errors.New("Kiro model catalog is empty or exceeds the safe size limit")
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return "", errors.New("failed to parse Kiro model catalog JSON")
	}
	models, ok := findModels(value)
	if !ok {
		return "", errors.New("Kiro model catalog is missing models array")
	}
	if len(models) > catalogHardLimit {
		return "", fmt.Errorf("Kiro model catalog exceeds the hard limit of %d entries", catalogHardLimit)
	}
	normalized := normalizeModels(models)
	if len(normalized) == 0 {
		return "", errors.New("Kiro model catalog returned no usable models")
	}
	content, err := json.MarshalIndent(map[string]any{"models": normalized}, "", "  ")
	if err != nil {
		return "", errors.New("failed to serialize Kiro model catalog")
	}
	if len(content) > secretMaxBytes {
		return "", errors.New("Kiro model catalog exceeds the safe size limit")
	}
	return string(content), nil
}

func normalizeModels(models []any) []map[string]any {
	seen := make(map[string]bool, len(models))
	result := make([]map[string]any, 0, len(models))
	for _, raw := range models {
		model, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := firstString(model, "id", "model_id", "modelId", "slug", "model")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		name := firstString(model, "name", "model_name", "modelName")
		if name == "" {
			name = id
		}
		item := map[string]any{
			"id": id, "name": name, "object": "model", "owned_by": "kiro-cli",
		}
		if description := firstString(model, "description"); description != "" {
			item["description"] = description
		}
		if contextWindow := firstPositiveUint64(model, "context_window_tokens", "contextWindowTokens"); contextWindow > 0 {
			item["context_window_tokens"] = contextWindow
		}
		result = append(result, item)
	}
	return result
}

func firstPositiveUint64(object map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		switch value := object[key].(type) {
		case float64:
			if value > 0 && value == float64(uint64(value)) {
				return uint64(value)
			}
		case json.Number:
			parsed, err := value.Int64()
			if err == nil && parsed > 0 {
				return uint64(parsed)
			}
		}
	}
	return 0
}

func findModels(value any) ([]any, bool) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	if models, ok := firstArray(root, "models", "availableModels", "available_models", "supportedModels", "supported_models"); ok {
		return models, true
	}
	nested, ok := root["models"].(map[string]any)
	if !ok {
		return nil, false
	}
	return firstArray(nested, "availableModels", "available_models", "supportedModels", "supported_models")
}

func firstArray(object map[string]any, keys ...string) ([]any, bool) {
	for _, key := range keys {
		value, ok := object[key].([]any)
		if ok && len(value) > 0 {
			return value, true
		}
	}
	return nil, false
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := object[key].(string)
		if ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func normalizedOptional(value *string) *string {
	if value == nil {
		return nil
	}
	return optional(strings.TrimSpace(*value))
}
