package profile

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

type fakeKiroSource struct {
	credential profilemodel.BuiltinCredential
	err        error
}

func (fake fakeKiroSource) Load(context.Context) (profilemodel.BuiltinCredential, error) {
	return fake.credential, fake.err
}

func TestImportBuiltinKiroCreatesUpdatesAndPreservesWarning(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), "current"))
	authKey, authKind := "kirocli:social:token", "social"
	profileARN, profileName := "arn:fixture", "upstream-main"
	catalog.SetKiroSource(fakeKiroSource{credential: profilemodel.BuiltinCredential{
		Provider: profilemodel.ProviderSnapshot{
			Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind,
			ProfileARN: &profileARN, ProfileName: &profileName,
		},
		Email: "person@example.test",
		SecretFiles: []profilemodel.ExportedSecretFile{
			{Path: kiroCredentialFile, Text: `{"auth_key":"kirocli:social:token","auth_kind":"social","auth_json":"{}"}`},
			{Path: kiroModelCatalogFile, Text: `{"models":[{"id":"old-model"}]}`},
		},
	}})

	created, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "kiro"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Profile != "kiro-person_example.test" || created.Provider != "kiro" || created.Updated || !created.Active {
		t.Fatalf("created = %+v", created)
	}
	stored, err := repo.Resolve(context.Background(), created.Profile)
	if err != nil || stored.Provider.AuthKey != authKey || stored.Provider.AuthKind != authKind || stored.Provider.ProfileARN != profileARN || stored.Provider.ProfileName != profileName || stored.Email != "person@example.test" {
		t.Fatalf("stored = %#v, err = %v", stored, err)
	}

	catalog.SetKiroSource(fakeKiroSource{credential: profilemodel.BuiltinCredential{
		Provider: profilemodel.ProviderSnapshot{
			Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind,
			ProfileARN: &profileARN, ProfileName: &profileName,
		},
		Email:       "updated@example.test",
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: kiroCredentialFile, Text: `{"auth_key":"kirocli:social:token","auth_kind":"social","auth_json":"{\"access_token\":\"new\"}"}`}},
		Warning:     "Kiro model catalog refresh failed; re-import this profile to retry.",
	}})
	updated, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "kiro"})
	if err != nil || !updated.Updated || updated.Profile != created.Profile || updated.Warning == "" {
		t.Fatalf("updated = %+v, err = %v", updated, err)
	}
	stored, err = repo.Resolve(context.Background(), created.Profile)
	if err != nil || stored.Email != "updated@example.test" {
		t.Fatalf("updated stored = %#v, err = %v", stored, err)
	}
	catalogText, found, err := repo.ReadOptionalProviderSecret(stored.CodexHome, kiroModelCatalogFile)
	if err != nil || !found || !strings.Contains(catalogText, "old-model") {
		t.Fatalf("catalog after failed refresh = %q, found=%t, err=%v", catalogText, found, err)
	}
}

func TestImportBuiltinKiroRejectsDifferentNameForExistingIdentity(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), "current"))
	authKey, authKind := "key", "builder-id"
	profileARN, profileName := "ARN:FIXTURE", "Main"
	credential := profilemodel.BuiltinCredential{
		Provider:    profilemodel.ProviderSnapshot{Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind, ProfileARN: &profileARN, ProfileName: &profileName},
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: kiroCredentialFile, Text: `{"auth_key":"key","auth_kind":"builder-id","auth_json":"{}"}`}},
	}
	catalog.SetKiroSource(fakeKiroSource{credential: credential})
	first, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "kiro", Name: "kiro-main"})
	if err != nil || first.Profile != "kiro-main" {
		t.Fatalf("first = %+v, err = %v", first, err)
	}
	lowerARN, lowerName := "arn:fixture", "main"
	credential.Provider.ProfileARN, credential.Provider.ProfileName = &lowerARN, &lowerName
	catalog.SetKiroSource(fakeKiroSource{credential: credential})
	if _, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "kiro", Name: "kiro-alt"}); err == nil || !strings.Contains(err.Error(), "already imported as profile") {
		t.Fatalf("mismatched name error = %v", err)
	}
	updated, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "kiro", Name: "kiro-main", Activate: true})
	if err != nil || !updated.Updated || !updated.Active {
		t.Fatalf("same-name update = %+v, err = %v", updated, err)
	}
}

func TestImportBuiltinKiroDefaultNameUsesUpstreamProfileAndUniqueSuffix(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(t.TempDir(), "current"))
	authKind := "builder-id"
	for index, fixture := range []struct {
		authKey, upstream, want string
	}{
		{"key-a", "Team Main", "kiro-team-main"},
		{"key-b", "Team Main", "kiro-team-main-2"},
	} {
		catalog.SetKiroSource(fakeKiroSource{credential: profilemodel.BuiltinCredential{
			Provider:    profilemodel.ProviderSnapshot{Kind: "kiro", AuthKey: &fixture.authKey, AuthKind: &authKind, ProfileName: &fixture.upstream},
			SecretFiles: []profilemodel.ExportedSecretFile{{Path: kiroCredentialFile, Text: `{"auth_key":"` + fixture.authKey + `","auth_kind":"builder-id","auth_json":"{}"}`}},
		}})
		result, err := catalog.ImportBuiltin(context.Background(), profilemodel.BuiltinImportRequest{Source: "kiro"})
		if err != nil || result.Profile != fixture.want || result.Updated {
			t.Fatalf("fixture %d result = %+v, err = %v", index, result, err)
		}
	}
}
