package redact

import (
	"strings"
	"testing"
)

func TestSecretsRedactsCredentialShapes(t *testing.T) {
	bearer := strings.Join([]string{"fixture", "token", "123"}, "_")
	apiKey := strings.Join([]string{"api", "sentinel", "value"}, "-")
	skToken := "sk-" + strings.Repeat("a", 24)
	value := "Authorization: Bearer " + bearer + " api_key=" + apiKey + " https://user:pass@example.test " + skToken
	got := Secrets(value)
	needles := []string{bearer, apiKey, "user:pass", skToken}
	for index := 0; index < len(needles); index++ {
		needle := needles[index]
		if strings.Contains(got, needle) {
			t.Fatalf("redacted value leaked %q: %q", needle, got)
		}
	}
	if !strings.Contains(got, "<redacted>") {
		t.Fatalf("redacted value = %q", got)
	}
}
