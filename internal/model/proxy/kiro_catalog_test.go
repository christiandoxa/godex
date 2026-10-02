package proxy

import "testing"

func TestKiroCatalogMatchesProdex04351(t *testing.T) {
	entries, err := KiroProviderCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("Kiro catalog count = %d", len(entries))
	}
	luna := ResolveProviderCatalogEntry(entries, "LUNA")
	if luna == nil || luna.ID != "gpt-5.6-luna" || luna.ContextWindowTokens == nil || *luna.ContextWindowTokens != 1_000_000 || luna.DefaultReasoningEffort == nil || *luna.DefaultReasoningEffort != "medium" {
		t.Fatalf("Luna entry = %#v", luna)
	}
	auto := ResolveProviderCatalogEntry(entries, "default")
	if auto == nil || auto.ID != "auto" || len(auto.SupportedEndpoints) != 5 {
		t.Fatalf("Auto entry = %#v", auto)
	}
}
