package main

import (
	"slices"
	"strings"
	"testing"
)

func TestProdex04380SuperPositionedProviderExecutableOracleRejectsWrongProvider(t *testing.T) {
	names := []string{}
	for _, tc := range cliProbes() {
		if tc.Provider == "" {
			continue
		}
		names = append(names, tc.Name)
		canonical := commandResult{Accepted: true, Output: "Provider: prodex-" + tc.Provider + "\n"}
		candidate := commandResult{Accepted: true, Output: "Provider: " + tc.Provider + "\nCodex args: safe\n"}
		if diff := compareProbe(tc, canonical, candidate); len(diff) != 0 {
			t.Fatalf("reference alias %q failed: %v", tc.Name, diff)
		}
		for _, altered := range []commandResult{
			{Accepted: true, Output: "Provider: openai\n"},
			{Accepted: true, Output: "Codex args: --provider " + tc.Provider + "\n"},
			{Accepted: true, Output: "Provider: " + tc.Provider + " extra\n"},
			{Accepted: false, ExitCode: 1, Output: "Provider: " + tc.Provider + "\n"},
		} {
			if diff := compareProbe(tc, canonical, altered); len(diff) == 0 {
				t.Fatalf("wrong provider or exit evaded oracle for %s: %+v", tc.Name, altered)
			}
		}
		wrongCanonical := canonical
		wrongCanonical.Output = "Provider: prodex-other\n"
		if diff := compareProbe(tc, wrongCanonical, candidate); len(diff) == 0 {
			t.Fatalf("wrong canonical selected provider escaped source oracle: %s", tc.Name)
		}
	}
	for _, want := range []string{"short_super_positioned_gemini", "short_super_positioned_deepseek",
		"super_positioned_gemini", "super_positioned_deepseek"} {
		if !slices.Contains(names, want) {
			t.Fatalf("missing tagged positioned provider probe %s", want)
		}
	}
	if value, ok := cliDryRunProvider04380("Codex args: Provider: gemini\n"); ok || value != "" {
		t.Fatal("native Codex tail was mistaken for provider selection")
	}
	if value, ok := cliDryRunProvider04380(strings.Repeat("log\n", 20) + " Provider: deepseek\n"); !ok || value != "deepseek" {
		t.Fatal("valid dry-run provider marker was lost")
	}
}
