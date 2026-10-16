package ping

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The Prodex 0.438.0 catalog_choices_v2 ABI caps the *total* count of alias
// and effort views at 65,536 each. A small models_cache.json can still
// contain >65,536 very short entries; byte-size limits alone are insufficient.
func TestProdex04380DynamicModelCacheRejectsExcessiveAliasAndEffortEntries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		makeEntry func() map[string]any
	}{
		{name: "alias-count 65537", makeEntry: func() map[string]any {
			aliases := make([]string, 65537)
			for i := range aliases {
				aliases[i] = "a"
			}
			return map[string]any{"slug": "synthetic-model", "aliases": aliases}
		}},
		{name: "effort-count 65537", makeEntry: func() map[string]any {
			levels := make([]map[string]string, 65537)
			for i := range levels {
				levels[i] = map[string]string{"effort": "x"}
			}
			return map[string]any{"slug": "synthetic-model", "supported_reasoning_levels": levels}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CODEX_HOME", home)
			encoded, err := json.Marshal(map[string]any{"models": []map[string]any{tc.makeEntry()}})
			if err != nil {
				t.Fatal(err)
			}
			if len(encoded) > pingModelCacheMaxBytes {
				t.Skipf("fixture size exceeds capped dynamic cache: %d", len(encoded))
			}
			if err := os.WriteFile(filepath.Join(home, "models_cache.json"), encoded, 0600); err != nil {
				t.Fatal(err)
			}
			if got := pingDynamicModels(); len(got) != 0 {
				t.Fatalf("exceeded tagged catalog input view budget, got %d models", len(got))
			}
		})
	}
}

func TestProdex04380DynamicModelCacheAcceptsExactly65536AliasViews(t *testing.T) {
	aliases := make([]string, 65536)
	for i := range aliases {
		aliases[i] = "a"
	}
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	encoded, err := json.Marshal(map[string]any{"models": []map[string]any{{
		"slug": "synthetic-model", "aliases": aliases,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > pingModelCacheMaxBytes {
		t.Fatal("valid exact-limit fixture exceeded file guard")
	}
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if got := pingDynamicModels(); len(got) != 1 || got[0].id != "synthetic-model" {
		t.Fatalf("exact tagged Mojo alias-view bound was rejected: got %d models", len(got))
	}
}

func TestProdex04380DynamicModelCacheTotalsAliasesAcrossModels(t *testing.T) {
	aliases := make([]string, 32769)
	for i := range aliases {
		aliases[i] = "a"
	}
	root := []map[string]any{
		{"slug": "first-model", "aliases": aliases[:32768]},
		{"slug": "second-model", "aliases": aliases},
	}
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	encoded, err := json.Marshal(map[string]any{"models": root})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > pingModelCacheMaxBytes {
		t.Fatal("cross-model alias fixture exceeded file guard")
	}
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if got := pingDynamicModels(); len(got) != 0 {
		t.Fatalf("per-model view guard missed aggregate overflow: %+v", got)
	}
}
