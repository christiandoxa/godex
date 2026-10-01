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

type fakeCopilotSource struct {
	credential profilemodel.BuiltinCredential
	err        error
}

func (fake fakeCopilotSource) Load(context.Context) (profilemodel.BuiltinCredential, error) {
	return fake.credential, fake.err
}

const (
	copilotMainProfileFixture = "copilot-main"
	copilotCurrentFixture     = "current"
)

func TestImportBuiltinCopilotCreatesMetadataOnlyProfile(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), copilotCurrentFixture))
	host := "https://github.example.test"
	resolvedLogin := "resolved-login"
	apiURL := "https://copilot-api.example.test"
	sku, plan := "enterprise", "business"
	catalog.SetCopilotSource(fakeCopilotSource{credential: profilemodel.BuiltinCredential{
		Provider: profilemodel.ProviderSnapshot{
			Kind: copilotProviderLabel, Host: &host, Login: &resolvedLogin, APIURL: &apiURL,
			AccessTypeSKU: &sku, CopilotPlan: &plan,
		},
		Email: "config-login",
	}})

	created, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: copilotProviderLabel})
	if err != nil {
		t.Fatal(err)
	}
	if created.Profile != "copilot-config-login" || created.Provider != copilotProviderLabel || created.Updated || !created.Active {
		t.Fatalf("created = %+v", created)
	}
	stored, err := repo.Resolve(context.Background(), created.Profile)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Email != "config-login" || stored.Provider.Host != host || stored.Provider.Login != resolvedLogin || stored.Provider.APIURL != apiURL || stored.Provider.AccessTypeSKU != sku || stored.Provider.CopilotPlan != plan {
		t.Fatalf("stored = %#v", stored)
	}
	if entries, err := filepath.Glob(filepath.Join(stored.CodexHome, "*")); err != nil || len(entries) != 0 {
		t.Fatalf("Copilot managed home unexpectedly contains secrets/files: %#v, err=%v", entries, err)
	}
}

func TestImportBuiltinCopilotUpdatesExistingIdentityAndRejectsDifferentName(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), copilotCurrentFixture))
	host, login := "https://github.com", "octocat"
	apiOld, apiNew := "https://api.githubcopilot.com", "https://copilot-api.example.test"
	planOld, planNew := "individual", "business"
	credential := profilemodel.BuiltinCredential{
		Provider: profilemodel.ProviderSnapshot{Kind: copilotProviderLabel, Host: &host, Login: &login, APIURL: &apiOld, CopilotPlan: &planOld},
		Email:    login,
	}
	catalog.SetCopilotSource(fakeCopilotSource{credential: credential})
	first, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: copilotProviderLabel, Name: copilotMainProfileFixture})
	if err != nil || first.Profile != copilotMainProfileFixture {
		t.Fatalf("first = %+v, err = %v", first, err)
	}

	credential.Provider.APIURL = &apiNew
	credential.Provider.CopilotPlan = &planNew
	catalog.SetCopilotSource(fakeCopilotSource{credential: credential})
	if _, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: copilotProviderLabel, Name: "copilot-alt"}); err == nil || !strings.Contains(err.Error(), "already imported as profile") {
		t.Fatalf("different-name error = %v", err)
	}
	updated, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: copilotProviderLabel, Name: copilotMainProfileFixture, Activate: true})
	if err != nil || !updated.Updated || !updated.Active || updated.Profile != copilotMainProfileFixture {
		t.Fatalf("updated = %+v, err = %v", updated, err)
	}
	stored, err := repo.Resolve(context.Background(), copilotMainProfileFixture)
	if err != nil || stored.Provider.APIURL != apiNew || stored.Provider.CopilotPlan != planNew || stored.Email != login {
		t.Fatalf("stored = %#v, err = %v", stored, err)
	}
}

func TestCopilotIdentityUsesTrimOnlyConfigLogin(t *testing.T) {
	listed := []Report{{Profile: profileentity.Profile{
		Name: "existing", Provider: profileentity.Provider{Kind: profileentity.ProviderCopilot, Host: " github.com ", Login: "Octocat"},
	}}}
	host, resolved := "github.com", "resolved-login"
	provider := profilemodel.ProviderSnapshot{Kind: copilotProviderLabel, Host: &host, Login: &resolved}
	if report, ok := findCopilotIdentity(listed, provider, " Octocat "); !ok || report.Profile.Name != "existing" {
		t.Fatalf("trimmed identity match = %+v, %t", report, ok)
	}
	if _, ok := findCopilotIdentity(listed, provider, "octocat"); ok {
		t.Fatal("case-folded Copilot login unexpectedly matched")
	}
}

func TestImportBuiltinCopilotUsesUniqueDefaultName(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), copilotCurrentFixture))
	for index, fixture := range []struct {
		host, login, want string
	}{
		{"https://github-a.example.test", "Team User", "copilot-team-user"},
		{"https://github-b.example.test", "Team User", "copilot-team-user-2"},
	} {
		host, login := fixture.host, fixture.login
		catalog.SetCopilotSource(fakeCopilotSource{credential: profilemodel.BuiltinCredential{
			Provider: profilemodel.ProviderSnapshot{Kind: copilotProviderLabel, Host: &host, Login: &login}, Email: login,
		}})
		result, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: copilotProviderLabel})
		if err != nil || result.Profile != fixture.want || result.Updated {
			t.Fatalf("fixture %d result = %+v, err = %v", index, result, err)
		}
	}
}
