package kiro

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestKiroInspectorParsesAuthSecret(t *testing.T) {
	text := kiroAuthFixture(t, map[string]any{
		"auth_key": "kirocli:social:token", "auth_kind": "social",
		"auth_json": `{"access_token":"fixture"}`, "email": "person@example.test",
		"profile_arn": "arn:fixture", "profile_name": "main",
		"start_url": "https://example.test/start", "region": "us-east-1",
	})
	credential, err := NewInspector().InspectAuthSecret(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	provider := credential.Provider
	if provider.Kind != "kiro" || provider.AuthKey == nil || *provider.AuthKey != "kirocli:social:token" || provider.AuthKind == nil || *provider.AuthKind != "social" || provider.ProfileARN == nil || *provider.ProfileARN != "arn:fixture" || provider.StartURL == nil || *provider.StartURL != "https://example.test/start" || credential.Email != "person@example.test" {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestKiroInspectorRejectsInvalidAuthSecret(t *testing.T) {
	fixtures := []string{
		`{}`,
		kiroAuthFixture(t, map[string]any{"auth_key": "key", "auth_kind": "kind", "auth_json": ""}),
		kiroAuthFixture(t, map[string]any{"auth_key": "key", "auth_kind": "kind", "auth_json": "{"}),
	}
	for _, text := range fixtures {
		if _, err := NewInspector().InspectAuthSecret(context.Background(), text); err == nil {
			t.Fatalf("invalid auth %q accepted", text)
		}
	}
}

func TestKiroInspectorValidatesCatalogShapesAndDedupes(t *testing.T) {
	inspector := NewInspector()
	for _, text := range []string{
		`{"models":[{"model_id":"snake-id"},{"modelId":"camel-id"},{"model_id":"snake-id"}]}`,
		`{"availableModels":[{"slug":"top-level-id"}]}`,
		`{"models":{"supportedModels":[{"model":"nested-id"}]}}`,
	} {
		text = strings.ReplaceAll(text, `\"`, `"`)
		if err := inspector.ValidateModelCatalog(context.Background(), text); err != nil {
			t.Fatalf("catalog %s: %v", text, err)
		}
	}
	invalid := strings.ReplaceAll(`{"models":[{"name":"missing-id"}]}`, `\"`, `"`)
	if err := inspector.ValidateModelCatalog(context.Background(), invalid); err == nil {
		t.Fatal("catalog without usable model accepted")
	}
}

func TestKiroInspectorRejectsOversizedCatalogCount(t *testing.T) {
	models := make([]string, 0, catalogHardLimit+1)
	for index := 0; index <= catalogHardLimit; index++ {
		models = append(models, fmt.Sprintf(`{"id":"model-%d"}`, index))
	}
	text := strings.ReplaceAll(`{"models":[`+strings.Join(models, ",")+`]}`, `\"`, `"`)
	if err := NewInspector().ValidateModelCatalog(context.Background(), text); err == nil || !strings.Contains(err.Error(), "hard limit") {
		t.Fatalf("oversized catalog error = %v", err)
	}
}

func kiroAuthFixture(t *testing.T, value map[string]any) string {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestProdex04356KiroCatalogNormalizationParity(t *testing.T) {
	input := []any{
		map[string]any{
			"id": " primary ", "model_id": "ignored-alias", "name": " ", "modelName": "Display",
			"description": "details", "context_window_tokens": float64(0), "contextWindowTokens": float64(456),
		},
		map[string]any{"modelId": "PRIMARY", "name": "duplicate"},
		map[string]any{"slug": "other", "description": ""},
	}
	models := normalizeModels(input)
	if len(models) != 2 {
		t.Fatalf("normalized model count = %d, want 2: %#v", len(models), models)
	}
	if models[0]["id"] != "primary" || models[0]["name"] != "Display" || models[0]["description"] != "details" || models[0]["context_window_tokens"] != uint64(456) {
		t.Fatalf("primary model = %#v", models[0])
	}
	if models[1]["id"] != "other" || models[1]["name"] != "other" {
		t.Fatalf("other model = %#v", models[1])
	}
	description, exists := models[1]["description"]
	if !exists || description != "" {
		t.Fatalf("empty description = %#v, exists=%t", description, exists)
	}
}

func TestProdex04356KiroCatalogAcceptsArrayRoot(t *testing.T) {
	text := `[{"model_id":"model-a","model_name":"Model A"}]`
	normalized, err := normalizeModelCatalogText(text)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(normalized), &value); err != nil {
		t.Fatal(err)
	}
	models, _ := value["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("normalized root-array models = %#v", models)
	}
	model, _ := models[0].(map[string]any)
	if model["id"] != "model-a" || model["name"] != "Model A" {
		t.Fatalf("normalized root-array model = %#v", model)
	}
}
