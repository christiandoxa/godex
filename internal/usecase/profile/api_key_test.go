package profile

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

func TestLoginAPIKeyCreatesAndUpdatesProdexNamedProfile(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), "current"))
	result, err := catalog.LoginAPIKey(context.Background(), profilemodel.APIKeyLoginInput{
		APIKey: "fixture-key", BaseURL: "https://Example.Test/v1/", BaseURLSpecified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Profile != "api_key_example.test" || !result.Created || !result.Active || result.BaseURL != "https://Example.Test/v1/" {
		t.Fatalf("create result = %+v", result)
	}
	stored, err := repo.Resolve(context.Background(), result.Profile)
	if err != nil || stored.Provider.Kind != profileentity.ProviderOpenAI || stored.Email != "" {
		t.Fatalf("stored = %#v, err=%v", stored, err)
	}
	auth, err := repo.ReadAuthJSON(stored.CodexHome)
	if err != nil || !strings.Contains(string(auth), `"auth_mode": "apikey"`) || !strings.Contains(string(auth), `"OPENAI_API_KEY": "fixture-key"`) {
		t.Fatalf("auth = %q, err=%v", auth, err)
	}
	clear(auth)

	updated, err := catalog.LoginAPIKey(context.Background(), profilemodel.APIKeyLoginInput{
		Name: result.Profile, APIKey: "replacement", BaseURLSpecified: false,
	})
	if err != nil || updated.Created || updated.Profile != result.Profile {
		t.Fatalf("update result = %+v, err=%v", updated, err)
	}
}

func TestLoginAPIKeyExplicitNameSanitizesAndRejectsNonOpenAICollision(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), "current"))
	anthropic := profileentity.Profile{
		Name: "api-key-work", CodexHome: repo.ManagedHome("api-key-work"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderAnthropic},
	}
	if err := repo.ImportProvider(context.Background(), anthropic, map[string]string{}, false); err != nil {
		t.Fatal(err)
	}
	_, err := catalog.LoginAPIKey(context.Background(), profilemodel.APIKeyLoginInput{Name: " API KEY WORK ", APIKey: "fixture"})
	if err == nil || !strings.Contains(err.Error(), "does not support Codex API-key login") {
		t.Fatalf("non-OpenAI collision error = %v", err)
	}
}

func TestLoginAPIKeyValidationNeverLeaksKeyOrURL(t *testing.T) {
	catalog := NewCatalog(profilerepo.NewStore(t.TempDir()), &fakeAccounts{}, filepath.Join(t.TempDir(), "current"))
	secretKey := "api-key-secret-sentinel"
	secretURL := "https://user:url-secret-sentinel@example.test/v1"
	_, err := catalog.LoginAPIKey(context.Background(), profilemodel.APIKeyLoginInput{
		APIKey: secretKey, BaseURL: secretURL, BaseURLSpecified: true,
	})
	if err == nil || strings.Contains(err.Error(), secretKey) || strings.Contains(err.Error(), "secret-sentinel") {
		t.Fatalf("unsafe validation error = %v", err)
	}
}
