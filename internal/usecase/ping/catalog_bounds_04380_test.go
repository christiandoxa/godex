package ping

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The 0.438.0 compiled rich_catalog.mojo plan validates the *entire* input
// batch before selecting eligible models. One oversized ID, label, effort or
// over-limit model count fails the dynamic cache plan; it must not silently
// keep a partial list from the same untrusted cache payload.
func TestProdex04380DynamicModelCacheFailsClosedOnMojoInputBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		models []map[string]any
	}{
		{"identifier oversized", []map[string]any{{"slug": "good"}, {"slug": strings.Repeat("x", 4097)}}},
		{"display label oversized", []map[string]any{{"slug": "good"}, {"slug": "too-big-label", "display_name": strings.Repeat("y", 65537)}}},
		{"effort oversized", []map[string]any{{"slug": "good"}, {"slug": "too-big-effort", "supported_reasoning_levels": []map[string]string{{"effort": strings.Repeat("z", 65537)}}}}},
		{"alias oversized", []map[string]any{{"slug": "good"}, {"slug": "too-big-alias", "aliases": []string{strings.Repeat("a", 4097)}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CODEX_HOME", home)
			encoded, err := json.Marshal(map[string]any{"models": tc.models})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "models_cache.json"), encoded, 0600); err != nil {
				t.Fatal(err)
			}
			if got := pingDynamicModels(); len(got) != 0 {
				t.Fatalf("invalid batch escaped Mojo input validation: %+v", got)
			}
		})
	}
}

func TestProdex04380DynamicModelCacheRejectsMoreThan1024InputModels(t *testing.T) {
	models := make([]map[string]string, 1025)
	for i := range models {
		models[i] = map[string]string{"slug": fmt.Sprintf("model-%04d", i)}
	}
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	encoded, err := json.Marshal(map[string]any{"models": models})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > pingModelCacheMaxBytes {
		t.Fatal("fixture exceeded outer JSON file-size guard")
	}
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if got := pingDynamicModels(); len(got) != 0 {
		t.Fatalf("Mojo 1024-model input limit ignored: got %d dynamic choices", len(got))
	}
}

func TestProdex04380DynamicModelCacheAcceptsValid5000ByteEffort(t *testing.T) {
	effort := strings.Repeat("x", 5000)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	encoded, err := json.Marshal(map[string]any{"models": []map[string]any{{
		"slug": "long-but-valid-effort", "supported_reasoning_levels": []map[string]string{{"effort": effort}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	models := pingDynamicModels()
	if len(models) != 1 || len(models[0].efforts) != 1 || models[0].efforts[0] != effort {
		t.Fatalf("valid Mojo effort range was incorrectly truncated: models=%d", len(models))
	}
}

func TestProdex04380DynamicCatalogUsesFirstStringFieldEvenWhenEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	const payload = `{"models":[
      {"slug":"","id":"skip-not-fallback","priority":1},
      {"slug":null,"id":"use-fallback-after-null","priority":2},
      {"slug":17,"id":"use-fallback-after-type","priority":3},
      {"slug":"keep-label-id","display_name":"","displayName":"wrong second label","priority":4},
      {"slug":"keep-null-label","display_name":null,"displayName":"use second label","priority":5}
    ]}`
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	got := pingDynamicModels()
	wantIDs := []string{"use-fallback-after-null", "use-fallback-after-type", "keep-label-id", "keep-null-label"}
	wantLabels := []string{"use-fallback-after-null", "use-fallback-after-type", "keep-label-id", "use second label"}
	if len(got) != len(wantIDs) {
		t.Fatalf("tagged first-string provider catalog: got=%+v", got)
	}
	for index, item := range got {
		if item.id != wantIDs[index] || item.label != wantLabels[index] {
			t.Errorf("catalog[%d]=%q/%q want %q/%q", index, item.id, item.label, wantIDs[index], wantLabels[index])
		}
	}
}

func TestProdex04380DynamicCatalogMixedTypesStillValidateStringBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		extra   map[string]any
		invalid bool
		efforts int
	}{
		{name: "oversized alias after numeric alias", extra: map[string]any{
			"aliases": []any{17, strings.Repeat("x", 4097)},
		}, invalid: true},
		{name: "oversized effort after non-object", extra: map[string]any{
			"supported_reasoning_levels": []any{17, map[string]any{"effort": strings.Repeat("x", 65537)}},
		}, invalid: true},
		{name: "valid effort after non-object", extra: map[string]any{
			"supported_reasoning_levels": []any{false, map[string]any{"effort": "medium"}},
		}, efforts: 1},
		{name: "malformed alias skipped but valid model preserved", extra: map[string]any{
			"aliases": []any{nil, false, "safe"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CODEX_HOME", home)
			entry := map[string]any{"slug": "synthetic-model"}
			for key, value := range tc.extra {
				entry[key] = value
			}
			encoded, err := json.Marshal(map[string]any{"models": []map[string]any{entry}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "models_cache.json"), encoded, 0600); err != nil {
				t.Fatal(err)
			}
			got := pingDynamicModels()
			if tc.invalid {
				if len(got) != 0 {
					t.Fatalf("oversized mixed-type batch was accepted: %+v", got)
				}
				return
			}
			if len(got) != 1 || len(got[0].efforts) != tc.efforts {
				t.Fatalf("valid mixed-type catalog lost typed effort: %+v", got)
			}
		})
	}
}
