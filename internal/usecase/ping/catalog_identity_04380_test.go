package ping

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Prodex 0.438.0 rich_catalog.mojo uses bytewise ASCII-only equality for
// model identity/choice de-duplication. Unicode Kelvin K and Latin k are
// DISTINCT identifiers; ASCII upper/lower should still deduplicate.
func TestProdex04380DynamicOpenAIModelIdentityUsesASCIIOnlyFold(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	const payload = `{"models":[
      {"slug":"book-model","priority":1},
      {"slug":"BOOK-MODEL","priority":2},
      {"slug":"booK-model","priority":3},
      {"slug":"BOOK-MODEL","priority":4},
      {"slug":"böok-model","priority":5},
      {"slug":"BÖOK-MODEL","priority":6}
    ]}`
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	choices := pingDynamicModels()
	got := make([]string, 0, len(choices))
	for _, model := range choices {
		got = append(got, model.id)
	}
	// ASCII case folds only each model's ASCII bytes. Unicode U+212A and
	// U+00D6 remain their original UTF-8 byte identities.
	want := []string{"book-model", "booK-model", "böok-model", "BÖOK-MODEL"}
	if !slices.Equal(got, want) {
		t.Fatalf("Mojo catalog model identity collision: got=%q want=%q", got, want)
	}
	for _, id := range want {
		found := pingDynamicModel(id)
		if found == nil || found.id != id {
			t.Fatalf("distinct dynamic model %q not selectable: %+v", id, found)
		}
	}
}

func TestProdex04380ASCIIModelComparisonChecksEveryUTF8Byte(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		equal bool
	}{
		{"BOOK", "book", true},
		{"book", "booK", false},
		{"BÖOK-MODEL", "böok-model", false},
		{"Aß", "aß", true},
		{"Ö", "ö", false},
		{"K", "k", false},
		{"é", "É", false},
		{"é", "é", false},
		{"ABC", "ABCD", false},
	} {
		if got := asciiEqualFold(tc.a, tc.b); got != tc.equal {
			t.Errorf("tagged ASCII byte comparison %q vs %q = %t, want %t", tc.a, tc.b, got, tc.equal)
		}
	}
}
