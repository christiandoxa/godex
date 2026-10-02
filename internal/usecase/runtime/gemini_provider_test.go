package runtime

import "testing"

func TestGeminiProviderMatchesProdexRuntimeDefaults(t *testing.T) {
	provider := GeminiProvider("gemini-api-key", "")
	if provider.Kind != "gemini" || provider.Name != "gemini-api-key" ||
		provider.APIURL != "https://generativelanguage.googleapis.com/v1beta" ||
		provider.DefaultModel != "auto" || provider.ContextWindow != 1_048_576 ||
		provider.AutoCompactLimit != 900_000 {
		t.Fatalf("Gemini provider = %#v", provider)
	}
	if custom := GeminiProvider("custom", " https://gemini.example.test/v1beta "); custom.APIURL != "https://gemini.example.test/v1beta" {
		t.Fatalf("custom Gemini base URL = %q", custom.APIURL)
	}
}
