package proxy

import "testing"

func TestDeepSeekCatalogMatchesProdex04343(t *testing.T) {
	entries, err := DeepSeekProviderCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("DeepSeek catalog len = %d", len(entries))
	}
	want := map[string][]string{
		"deepseek-v4-pro":   {"pro", "auto"},
		"deepseek-v4-flash": {"flash"},
		"deepseek-chat":     nil,
		"deepseek-reasoner": nil,
	}
	for _, entry := range entries {
		aliases, ok := want[entry.ID]
		if !ok {
			t.Fatalf("unexpected DeepSeek catalog entry %#v", entry)
		}
		if entry.Provider != "deepseek" || entry.OwnedBy != "deepseek" ||
			entry.ContextWindowTokens == nil || *entry.ContextWindowTokens != 128000 ||
			len(entry.SupportedEndpoints) != 5 || !entry.FeatureFlags["tools"].(bool) ||
			!entry.FeatureFlags["web_search"].(bool) || !entry.FeatureFlags["reasoning"].(bool) {
			t.Fatalf("DeepSeek entry metadata = %#v", entry)
		}
		if len(entry.Aliases) != len(aliases) {
			t.Fatalf("aliases for %s = %#v", entry.ID, entry.Aliases)
		}
	}
}
