package ping

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPingDynamicModelsTreatsMissingSupportedInAPIAsSupported(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	cache := "{\"models\":[{\"slug\":\"gpt-dynamic\",\"display_name\":\"Dynamic\",\"priority\":1,\"visibility\":\"list\",\"supported_reasoning_levels\":[{\"effort\":\"medium\"},{\"effort\":\"max\"}]}]}"
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	models := pingDynamicModels()
	if len(models) != 1 || models[0].id != "gpt-dynamic" {
		t.Fatalf("dynamic models = %#v, want gpt-dynamic", models)
	}
	if got := models[0].efforts; len(got) != 2 || got[0] != "medium" || got[1] != "max" {
		t.Fatalf("dynamic efforts = %#v", got)
	}
}

func TestPingDynamicModelsRejectsExplicitUnsupportedOrHidden(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	cache := "{\"models\":[{\"slug\":\"unsupported\",\"supported_in_api\":false,\"visibility\":\"list\"},{\"slug\":\"hidden\",\"supported_in_api\":true,\"hidden\":true,\"visibility\":\"list\"},{\"slug\":\"visible\",\"supported_in_api\":true,\"visibility\":\"list\"}]}"
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	models := pingDynamicModels()
	if len(models) != 1 || models[0].id != "visible" {
		t.Fatalf("dynamic models = %#v, want only visible", models)
	}
}

func TestNormalizePingEffortFallsBackToProviderDefaultForUnknownModel(t *testing.T) {
	got, err := normalizePingEffort("gpt-dynamic-only", "MAX")
	if err != nil {
		t.Fatal(err)
	}
	if got != "max" {
		t.Fatalf("normalized effort = %q, want max", got)
	}
	if _, err := normalizePingEffort("gpt-dynamic-only", "ultra"); err == nil {
		t.Fatal("provider-default unsupported effort unexpectedly accepted")
	}
}
