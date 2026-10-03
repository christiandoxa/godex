package gemini

import (
	"context"
	"strings"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func TestOAuthProfileQuotaMatchesDisabledProdexSurface(t *testing.T) {
	_, err := (ProfileQuota{}).FetchQuota(context.Background(), profilemodel.QuotaTarget{
		Name: "legacy-gemini", Provider: "gemini",
	})
	if err == nil || err.Error() != oauthProfileDisabled {
		t.Fatalf("Gemini OAuth profile quota error = %v", err)
	}
	if strings.Contains(err.Error(), "token") {
		t.Fatalf("Gemini OAuth profile quota leaked credential context: %v", err)
	}
}
