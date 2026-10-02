package runtime

import "testing"

func TestDeepSeekProviderMatchesProdexRuntimeDefaults(t *testing.T) {
	provider := DeepSeekProvider("deepseek-api-key", "")
	if provider.Kind != "deepseek" || provider.Name != "deepseek-api-key" ||
		provider.APIURL != "https://api.deepseek.com" ||
		provider.DefaultModel != "deepseek-v4-pro" ||
		provider.ContextWindow != 1_048_576 || provider.AutoCompactLimit != 900_000 {
		t.Fatalf("DeepSeek provider = %#v", provider)
	}
	custom := DeepSeekProvider("custom", "https://deepseek.example.test/v1")
	if custom.APIURL != "https://deepseek.example.test/v1" {
		t.Fatalf("custom DeepSeek base URL = %q", custom.APIURL)
	}
}

func TestDeepSeekProviderRuntimeArgumentsDoNotInjectExternalCatalog(t *testing.T) {
	provider := DeepSeekProvider("deepseek", "")
	arguments := []string{"exec", "hello"}
	prepared, err := prepareProviderRuntimeArguments(nil, t.TempDir(), provider, arguments)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, argument := range prepared {
		joined += argument + "\n"
	}
	for _, expected := range []string{
		`model="deepseek-v4-pro"`,
		"model_context_window=1048576",
		"model_auto_compact_token_limit=900000",
	} {
		if !containsText(joined, expected) {
			t.Fatalf("prepared args missing %q: %#v", expected, prepared)
		}
	}
	if containsText(joined, "model_catalog_json=") {
		t.Fatalf("DeepSeek unexpectedly injected external model catalog: %#v", prepared)
	}
}

func containsText(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
