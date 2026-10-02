package proxy

import "testing"

func TestCanonicalProviderCatalogsMatchProdex04351(t *testing.T) {
	anthropic, err := AnthropicProviderCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(anthropic) != 12 {
		t.Fatalf("Anthropic catalog count = %d", len(anthropic))
	}
	if entry := ResolveProviderCatalogEntry(anthropic, " DEFAULT "); entry == nil || entry.ID != "auto" {
		t.Fatalf("Anthropic alias lookup = %#v", entry)
	}
	if entry := ResolveProviderCatalogEntry(anthropic, "claude-sonnet-5-5"); entry == nil || entry.ContextWindowTokens == nil || *entry.ContextWindowTokens != 1_000_000 {
		t.Fatalf("Anthropic Sonnet 5.5 = %#v", entry)
	}

	copilot, err := CopilotProviderCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(copilot) != 30 {
		t.Fatalf("Copilot catalog count = %d", len(copilot))
	}
	for alias, id := range map[string]string{"astra": "gpt-6-astra", " SOL ": "gpt-6.1-sol", "luna": "gpt-6-luna"} {
		entry := ResolveProviderCatalogEntry(copilot, alias)
		if entry == nil || entry.ID != id {
			t.Fatalf("Copilot alias %q = %#v, want %q", alias, entry, id)
		}
	}
	if entry := ResolveProviderCatalogEntry(copilot, "gpt-5.1-codex"); entry != nil {
		t.Fatalf("retired Copilot static model remained: %#v", entry)
	}
}
