package profile

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

type fakeClaudeSource struct {
	credential profilemodel.BuiltinCredential
	err        error
}

func (fake fakeClaudeSource) Load(context.Context) (profilemodel.BuiltinCredential, error) {
	return fake.credential, fake.err
}

func TestImportBuiltinClaudeCreatesUpdatesAndSupportsNamedDuplicate(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), "current"))
	account := "person@example.test"
	method := "claude-ai-oauth:pro"
	catalog.SetClaudeSource(fakeClaudeSource{credential: profilemodel.BuiltinCredential{
		Provider:    profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &account, AuthMethod: &method},
		Email:       account,
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: ".credentials.json", Text: `{"accessToken":"old"}`}},
	}})

	created, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Profile != "claude-person_example.test" || created.Provider != "anthropic" || created.Updated || !created.Active {
		t.Fatalf("created = %+v", created)
	}
	profile, err := repo.Resolve(context.Background(), created.Profile)
	if err != nil || profile.Provider.Account != account || profile.Provider.AuthMethod != method || profile.Email != account {
		t.Fatalf("profile = %#v, err = %v", profile, err)
	}

	catalog.SetClaudeSource(fakeClaudeSource{credential: profilemodel.BuiltinCredential{
		Provider:    profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &account, AuthMethod: &method},
		Email:       account,
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: ".credentials.json", Text: `{"accessToken":"new"}`}},
	}})
	updated, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "claude"})
	if err != nil || !updated.Updated || updated.Profile != created.Profile {
		t.Fatalf("updated = %+v, err = %v", updated, err)
	}
	secret, err := repo.ReadProviderSecret(profile.CodexHome, ".credentials.json")
	if err != nil || !strings.Contains(secret, "new") {
		t.Fatalf("updated secret = %q, err = %v", secret, err)
	}

	duplicate, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "claude", Name: "claude-copy", Activate: true})
	if err != nil || duplicate.Updated || duplicate.Profile != "claude-copy" || !duplicate.Active {
		t.Fatalf("named duplicate = %+v, err = %v", duplicate, err)
	}
	listed, err := catalog.List(context.Background())
	if err != nil || len(listed) != 2 {
		t.Fatalf("listed = %+v, err = %v", listed, err)
	}
}

func TestImportBuiltinClaudeUsesUniqueFallbackNamesAndValidatesName(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), "current"))
	method := "claude-ai-oauth"
	catalog.SetClaudeSource(fakeClaudeSource{credential: profilemodel.BuiltinCredential{
		Provider:    profilemodel.ProviderSnapshot{Kind: "anthropic", AuthMethod: &method},
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: ".credentials.json", Text: `{"accessToken":"fixture"}`}},
	}})
	first, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "claude", Name: "claude"})
	if err != nil || first.Profile != "claude" {
		t.Fatalf("first = %+v, err = %v", first, err)
	}
	otherMethod := "claude-ai-oauth:max"
	catalog.SetClaudeSource(fakeClaudeSource{credential: profilemodel.BuiltinCredential{
		Provider:    profilemodel.ProviderSnapshot{Kind: "anthropic", AuthMethod: &otherMethod},
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: ".credentials.json", Text: `{"accessToken":"other"}`}},
	}})
	second, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "claude"})
	if err != nil || second.Profile != "claude-2" || second.Updated {
		t.Fatalf("second = %+v, err = %v", second, err)
	}
	if _, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "claude", Name: "bad/name"}); err == nil {
		t.Fatal("invalid requested name accepted")
	}
}

func TestSanitizeProfileSlugMatchesProdexRules(t *testing.T) {
	for input, want := range map[string]string{
		"Claude-Person@Example.Test": "claude-person_example.test",
		"  @@@  ":                    "profile",
		"a b/c":                      "a-b-c",
	} {
		if got := sanitizeProfileSlug(input); got != want {
			t.Fatalf("sanitize(%q) = %q, want %q", input, got, want)
		}
	}
}
