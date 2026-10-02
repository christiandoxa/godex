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
