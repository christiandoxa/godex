package runtime

import (
	"slices"
	"testing"
)

// Prodex 0.438.0 crates/prodex-cli/src/lib.rs calls the Mojo positioned
// provider alias *after* Clap parses leading Super flags. The first
// remaining Codex arg may then select gemini/deepseek, unless a literal
// boundary, explicit provider or explicit URL already owns that position.
func TestProdex04380SuperPositionedProviderAliasAfterOptions(t *testing.T) {
	cases := []struct {
		name      string
		arguments []string
		provider  string
		localURL  string
		codex     []string
	}{
		{"gemini after dry-run", []string{"--dry-run", "gemini"}, "gemini", "", nil},
		{"deepseek after dry-run", []string{"--dry-run", "deepseek"}, "deepseek", "", nil},
		{"deepseek after parsed model", []string{"--model", "synthetic-model", "--dry-run", "deepseek"}, "deepseek", "", nil},
		{"gemini after no presidio", []string{"--no-presidio", "--dry-run", "gemini"}, "gemini", "", nil},
		{"gemini before further codex input", []string{"--dry-run", "gemini", "exec", "review"}, "gemini", "", []string{"exec", "review"}},
		{"first codex positional is not provider", []string{"--dry-run", "exec", "gemini"}, "", "", []string{"-c", "features.apps=false", "exec", "gemini"}},
		{"explicit provider owns alias text", []string{"--dry-run", "--provider", "deepseek", "gemini"}, "deepseek", "", []string{"gemini"}},
		{"explicit local URL owns alias text", []string{"--dry-run", "--url", "http://127.0.0.1:1/v1", "gemini"}, "", "http://127.0.0.1:1/v1", []string{"gemini"}},
		{"literal boundary holds provider word", []string{"--dry-run", "--", "gemini"}, "", "", []string{"-c", "features.apps=false", "--", "gemini"}},
		{"literal boundary blocks initial shorthand", []string{"gemini", "--", "exec"}, "", "", []string{"-c", "features.apps=false", "gemini", "--", "exec"}},
		{"legacy first provider alias still works", []string{"gemini", "--dry-run"}, "gemini", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options, err := parseSuperArguments(tc.arguments)
			if err != nil {
				t.Fatal(err)
			}
			if options.provider != tc.provider || options.localURL != tc.localURL ||
				options.skipQuota != (tc.provider != "" || tc.localURL != "") {
				t.Fatalf("super provider rewrite wrong: provider=%q url=%q skip=%t, want provider=%q url=%q", options.provider, options.localURL, options.skipQuota, tc.provider, tc.localURL)
			}
			// superArguments may contain unrelated safety flags before
			// codexArgs; the suffix owned by the user must be preserved.
			got := options.codexArgs
			if tc.codex == nil {
				for _, arg := range got {
					if arg == "gemini" || arg == "deepseek" {
						t.Fatalf("positioned provider leaked into native Codex tail: %v", got)
					}
				}
				return
			}
			if !slices.Equal(got, tc.codex) {
				t.Fatalf("codex tail=%q, want %q", got, tc.codex)
			}
		})
	}
}
