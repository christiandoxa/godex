package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimerepo "github.com/christiandoxa/godex/internal/repository/runtime"
)

func TestAnthropicCatalogUsesProdexDefaultsAndStaticSeeds(t *testing.T) {
	home := t.TempDir()
	store := runtimerepo.NewProviderCatalogStore()
	provider := AnthropicProvider("claude", "")
	prepared, err := prepareProviderRuntimeArguments(store, home, provider, []string{"exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(prepared, " ")
	for _, expected := range []string{
		`model="claude-sonnet-5-5"`,
		"model_context_window=1000000",
		"model_auto_compact_token_limit=950000",
		"model_catalog_json=",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("prepared args missing %q: %#v", expected, prepared)
		}
	}
	models := readAnthropicCatalogModels(t, home)
	if len(models) != 12 || models[0]["slug"] != "claude-sonnet-5-5" || models[0]["context_window"] != float64(1000000) || models[0]["auto_compact_token_limit"] != float64(950000) {
		t.Fatalf("Anthropic catalog = %#v", models)
	}
	if findAnthropicModel(t, models, "auto")["display_name"] != "Anthropic Auto" || findAnthropicModel(t, models, "claude-opus-5-5")["display_name"] != "Claude Opus 5.5" {
		t.Fatalf("Anthropic seed metadata = %#v", models)
	}
}

func TestAnthropicCatalogPutsLaunchModelFirstAndRespectsOverrides(t *testing.T) {
	home := t.TempDir()
	store := runtimerepo.NewProviderCatalogStore()
	provider := AnthropicProvider("claude", "")
	arguments := []string{
		"--model", "custom-model",
		"-c", "model_context_window=300000",
		"-c", "model_auto_compact_token_limit=250000",
		"exec", "hello",
	}
	if _, err := prepareProviderRuntimeArguments(store, home, provider, arguments); err != nil {
		t.Fatal(err)
	}
	models := readAnthropicCatalogModels(t, home)
	if models[0]["slug"] != "custom-model" || models[0]["context_window"] != float64(300000) || models[0]["auto_compact_token_limit"] != float64(250000) {
		t.Fatalf("launch model = %#v", models[0])
	}
	if len(models) != 13 {
		t.Fatalf("catalog len = %d", len(models))
	}
}

func TestAnthropicCatalogRespectsUserCatalogOverride(t *testing.T) {
	home := t.TempDir()
	store := runtimerepo.NewProviderCatalogStore()
	provider := AnthropicProvider("claude", "")
	arguments := []string{"-c", `model_catalog_json="/tmp/custom.json"`, "exec", "hello"}
	prepared, err := prepareProviderRuntimeArguments(store, home, provider, arguments)
	if err != nil {
		t.Fatal(err)
	}
	managed := prepared[:len(prepared)-len(arguments)]
	if containsConfig(managed, "model_catalog_json=") {
		t.Fatalf("managed catalog unexpectedly injected: %#v", prepared)
	}
	if _, err := os.Lstat(filepath.Join(home, proxymodel.ExternalProviderCatalogFile)); !os.IsNotExist(err) {
		t.Fatalf("external catalog unexpectedly exists: %v", err)
	}
}

func readAnthropicCatalogModels(t *testing.T, home string) []map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(home, proxymodel.ExternalProviderCatalogFile))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Models
}

func findAnthropicModel(t *testing.T, models []map[string]any, slug string) map[string]any {
	t.Helper()
	for _, model := range models {
		if model["slug"] == slug {
			return model
		}
	}
	t.Fatalf("Anthropic catalog model %q not found", slug)
	return nil
}
