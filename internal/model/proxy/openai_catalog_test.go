package proxy

import "testing"

func TestOpenAIProviderCatalogMatchesProdex04351(t *testing.T) {
	entries, err := OpenAIProviderCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 11 {
		t.Fatalf("OpenAI catalog count = %d, want 11", len(entries))
	}
	for query, want := range map[string]struct {
		id      string
		context uint64
	}{
		"gpt-6-luna":  {id: "gpt-6-luna", context: 1_050_000},
		" SOL ":       {id: "gpt-6.1-sol", context: 1_050_000},
		"gpt-5.6-sol": {id: "gpt-5.6-sol", context: 872_000},
		"codex":       {id: "gpt-5.3-codex", context: 400_000},
	} {
		entry := ResolveProviderCatalogEntry(entries, query)
		if entry == nil || entry.ID != want.id || entry.ContextWindowTokens == nil || *entry.ContextWindowTokens != want.context {
			t.Fatalf("OpenAI model %q = %#v, want %q/%d", query, entry, want.id, want.context)
		}
	}
}

func TestProviderCatalogResolverUsesProdexASCIIOnlyCaseFold(t *testing.T) {
	entries, err := OpenAIProviderCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if entry := ResolveProviderCatalogEntry(entries, "ſol"); entry != nil {
		t.Fatalf("Unicode fold unexpectedly resolved ASCII alias: %#v", entry)
	}
	if entry := ResolveProviderCatalogEntry(entries, " SOL "); entry == nil || entry.ID != "gpt-6.1-sol" {
		t.Fatalf("ASCII casefold/trim regression: %#v", entry)
	}
}
