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

func TestProdex04355CopilotProfilelessAPIKeyProviderUsesPublicDefaultURL(t *testing.T) {
	provider := CopilotProvider("copilot-api-key", "", "", "")
	if provider.APIURL != "https://api.githubcopilot.com" || provider.DefaultModel != "gpt-6-astra" {
		t.Fatalf("Copilot API-key provider = %#v", provider)
	}
}

func TestCopilotCatalogUsesPromptLimitWithoutRuntimeSnapshot(t *testing.T) {
	home := t.TempDir()
	store := runtimerepo.NewProviderCatalogStore()
	provider := CopilotProvider("copilot", "github.com", "octocat", "https://api.githubcopilot.com")
	arguments := []string{"--model", "gpt-5.5", "exec", "hello"}
	prepared, err := prepareProviderRuntimeArguments(store, home, provider, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if !containsConfig(prepared, "model_catalog_json=") {
		t.Fatalf("prepared args missing catalog: %#v", prepared)
	}
	models := readExternalCatalogModels(t, home)
	if models[0]["slug"] != "gpt-5.5" || models[0]["context_window"] != float64(922000) || models[0]["auto_compact_token_limit"] != float64(875900) {
		t.Fatalf("gpt-5.5 catalog = %#v", models[0])
	}
	if got := strings.Join(reasoningEfforts(models[0]), ","); got != "low,medium,high,xhigh" {
		t.Fatalf("reasoning efforts = %s", got)
	}
}

func TestCopilotCatalogUsesDynamicPromptLimitAndStaticReasoning(t *testing.T) {
	home := t.TempDir()
	store := runtimerepo.NewProviderCatalogStore()
	dynamic := []map[string]any{
		{"id": "account/model-v1", "capabilities": map[string]any{"limits": map[string]any{"max_context_window_tokens": float64(1_050_000), "max_prompt_tokens": float64(333_000)}}},
		{"id": "gpt-5.6-luna", "context_window": float64(1_000_000)},
	}
	if _, err := store.WriteCopilotRuntime(home, dynamic); err != nil {
		t.Fatal(err)
	}
	provider := CopilotProvider("copilot", "github.com", "octocat", "https://api.githubcopilot.com")
	arguments := []string{"-c", `model="account/model-v1"`, "exec", "hello"}
	if _, err := prepareProviderRuntimeArguments(store, home, provider, arguments); err != nil {
		t.Fatal(err)
	}
	models := readExternalCatalogModels(t, home)
	if models[0]["slug"] != "account/model-v1" || models[0]["context_window"] != float64(333000) || models[0]["auto_compact_token_limit"] != float64(316350) {
		t.Fatalf("dynamic launch model = %#v", models[0])
	}
	luna := findCatalogModel(t, models, "gpt-5.6-luna")
	if luna["default_reasoning_level"] != "medium" {
		t.Fatalf("luna default reasoning = %#v", luna["default_reasoning_level"])
	}
	if got := strings.Join(reasoningEfforts(luna), ","); got != "none,low,medium,high,xhigh,max" {
		t.Fatalf("luna efforts = %s", got)
	}
}

func TestCopilotCatalogRespectsUserCatalogOverride(t *testing.T) {
	home := t.TempDir()
	store := runtimerepo.NewProviderCatalogStore()
	provider := CopilotProvider("copilot", "github.com", "octocat", "https://api.githubcopilot.com")
	arguments := []string{"-c", `model_catalog_json="/tmp/custom.json"`, "exec", "hello"}
	prepared, err := prepareProviderRuntimeArguments(store, home, provider, arguments)
	if err != nil {
		t.Fatal(err)
	}
	managed := prepared[:len(prepared)-len(arguments)]
	if containsConfig(managed, "model_catalog_json=") {
		t.Fatalf("Godex injected catalog despite user override: %#v", prepared)
	}
	if _, err := os.Lstat(filepath.Join(home, proxymodel.ExternalProviderCatalogFile)); !os.IsNotExist(err) {
		t.Fatalf("external catalog unexpectedly exists: %v", err)
	}
}

func TestProviderArgumentParsingUsesLastOverride(t *testing.T) {
	provider := CopilotProvider("copilot", "github.com", "octocat", "")
	arguments := []string{
		"--model", "first",
		"-c", `model="second"`,
		"--model=third",
		"-c", "model_context_window=400000",
		"-cmodel_auto_compact_token_limit=380000",
		"exec", "hello",
	}
	if got := effectiveProviderModel(provider, arguments); got != "third" {
		t.Fatalf("model = %q", got)
	}
	if got, err := effectiveProviderUint(arguments, "model_context_window", 1); err != nil || got != 400000 {
		t.Fatalf("context = %d, %v", got, err)
	}
	if got, err := effectiveProviderUint(arguments, "model_auto_compact_token_limit", 1); err != nil || got != 380000 {
		t.Fatalf("compact = %d, %v", got, err)
	}
	if _, err := effectiveProviderUint([]string{"-c", "model_context_window=-1"}, "model_context_window", 1); err == nil {
		t.Fatal("invalid provider integer unexpectedly accepted")
	}
}

func readExternalCatalogModels(t *testing.T, home string) []map[string]any {
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
	if len(envelope.Models) == 0 {
		t.Fatal("external provider catalog is empty")
	}
	return envelope.Models
}

func containsConfig(arguments []string, prefix string) bool {
	for _, argument := range arguments {
		if strings.HasPrefix(argument, prefix) {
			return true
		}
	}
	return false
}

func findCatalogModel(t *testing.T, models []map[string]any, slug string) map[string]any {
	t.Helper()
	for _, model := range models {
		if model["slug"] == slug {
			return model
		}
	}
	t.Fatalf("catalog model %q not found", slug)
	return nil
}

func reasoningEfforts(model map[string]any) []string {
	values, _ := model["supported_reasoning_levels"].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		entry, _ := value.(map[string]any)
		if effort, ok := entry["effort"].(string); ok {
			result = append(result, effort)
		}
	}
	return result
}

func TestCopilotStaticCatalogMatchesProdex04351Hotfix(t *testing.T) {
	home := t.TempDir()
	store := runtimerepo.NewProviderCatalogStore()
	provider := CopilotProvider("copilot", "github.com", "octocat", "https://api.githubcopilot.com")
	if _, err := prepareProviderRuntimeArguments(store, home, provider, []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	models := readExternalCatalogModels(t, home)
	for _, id := range []string{"gpt-6-astra", "gpt-6.1-sol", "gpt-6-luna", "gpt-6-sol", "claude-sonnet-5-5", "gemini-3.8-flash"} {
		_ = findCatalogModel(t, models, id)
	}
	for _, retired := range []string{"gpt-5.1-codex", "gemini-3.1-pro-preview"} {
		if catalogContainsModel(models, retired) {
			t.Fatalf("retired static Copilot model %q remained in catalog", retired)
		}
	}
	astra := findCatalogModel(t, models, "gpt-6-astra")
	if astra["context_window"] != float64(1_050_000) || astra["auto_compact_token_limit"] != float64(997_500) {
		t.Fatalf("Astra catalog = %#v", astra)
	}
	legacy := findCatalogModel(t, models, "gpt-5.3-codex")
	if legacy["context_window"] != float64(272_000) || legacy["auto_compact_token_limit"] != float64(258_400) {
		t.Fatalf("legacy prompt-limit override drifted: %#v", legacy)
	}
}

func catalogContainsModel(models []map[string]any, slug string) bool {
	for _, model := range models {
		if value, _ := model["slug"].(string); value == slug {
			return true
		}
	}
	return false
}
