package runtime

import (
	"strings"
	"testing"
)

func TestExternalAPIKeyProviderSupportsDeepSeek(t *testing.T) {
	provider, err := externalAPIKeyProvider("deepseek", "deepseek-api-key", "")
	if err != nil {
		t.Fatal(err)
	}
	if provider.Kind != "deepseek" || provider.APIURL != "https://api.deepseek.com" ||
		provider.DefaultModel != "deepseek-v4-pro" || provider.ContextWindow != 1_048_576 ||
		provider.AutoCompactLimit != 900_000 {
		t.Fatalf("DeepSeek provider = %#v", provider)
	}
	if err := providerCredentialRequired("deepseek"); err == nil || !strings.Contains(err.Error(), "DEEPSEEK_API_KEY(S)") {
		t.Fatalf("DeepSeek missing credential error = %v", err)
	}
}

func TestExternalAPIKeyProviderSupportsGemini(t *testing.T) {
	provider, err := externalAPIKeyProvider("gemini", "gemini-api-key", "")
	if err != nil {
		t.Fatal(err)
	}
	if provider.Kind != "gemini" || provider.APIURL != "https://generativelanguage.googleapis.com/v1beta" ||
		provider.DefaultModel != "auto" || provider.ContextWindow != 1_048_576 ||
		provider.AutoCompactLimit != 900_000 {
		t.Fatalf("Gemini provider = %#v", provider)
	}
	if err := providerCredentialRequired("gemini"); err == nil || !strings.Contains(err.Error(), "GOOGLE_API_KEY(S)") {
		t.Fatalf("Gemini missing credential error = %v", err)
	}
}
