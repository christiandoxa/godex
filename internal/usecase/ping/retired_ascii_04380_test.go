package ping

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Tagged Prodex 0.438.0 rich_catalog.mojo catalog_openai_model_is_retired
// trims Unicode whitespace but folds only ASCII A-Z. Unicode simple lower-
// casing can transmute Kelvin K into k, which must NOT retire a distinct ID.
func TestProdex04380RetiredOpenAIModelOnlyFoldsASCII(t *testing.T) {
	cases := []struct {
		name    string
		retired bool
	}{
		{"spark", true}, {"SPARK", true}, {" \u2003GPT-5.3-CODEX-SPARK\t", true},
		{"gpt-5.3-spark", true}, {"gPt-5.3-CoDeX-SpArK", true},
		{"sparK", false}, {"SPARK", false},
		{"gpt-5.3-codex-sparK", false},
		{"gpt-5.3-sparK", false},
		{"Kpt-5.3-spark", false},
		{"spark-extra", false}, {"", false}, {"\u2003", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pingRetiredModel(tc.name); got != tc.retired {
				t.Fatalf("0.438.0 ASCII retirement model=%q got=%t want=%t", tc.name, got, tc.retired)
			}
		})
	}
}

func TestProdex04380ModelPickerRetainsNonASCIILookalikesButHidesRetired(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	const models = `{"models":[
      {"slug":"spark","priority":1},
      {"slug":"SPARK","priority":2},
      {"slug":"gpt-5.3-CODEX-SPARK","priority":3},
      {"slug":"gpt-5.3-spark","priority":4},
      {"slug":"sparK","priority":5},
      {"slug":"gpt-5.3-codex-sparK","priority":6},
      {"slug":"gpt-5.3-sparK","priority":7},
      {"slug":"safe-model","priority":8}]}`
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(models), 0600); err != nil {
		t.Fatal(err)
	}
	got := pingDynamicModels()
	ids := make([]string, 0, len(got))
	for _, m := range got {
		ids = append(ids, m.id)
	}
	want := []string{"sparK", "gpt-5.3-codex-sparK", "gpt-5.3-sparK", "safe-model"}
	if !slices.Equal(ids, want) {
		t.Fatalf("openai model picker discarded distinct Unicode lookalikes: got=%q want=%q", ids, want)
	}
}
