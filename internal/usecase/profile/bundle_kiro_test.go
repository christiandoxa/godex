package profile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

type bundleKiroInspector struct {
	credentials map[string]profilemodel.BuiltinCredential
	catalogs    map[string]bool
}

func (inspector bundleKiroInspector) InspectAuthSecret(_ context.Context, text string) (profilemodel.BuiltinCredential, error) {
	credential, ok := inspector.credentials[text]
	if !ok {
		return profilemodel.BuiltinCredential{}, errors.New("invalid Kiro auth fixture")
	}
	return credential, nil
}

func (inspector bundleKiroInspector) ValidateModelCatalog(_ context.Context, text string) error {
	if !inspector.catalogs[text] {
		return errors.New("invalid Kiro catalog fixture")
	}
	return nil
}

func TestKiroBundleRoundTripWithOptionalCatalog(t *testing.T) {
	for _, password := range []string{"", "bundle-password"} {
		t.Run(boolLabel(password != ""), func(t *testing.T) {
			assertKiroBundleRoundTrip(t, password)
		})
	}
}

type kiroBundleFixture struct {
	authSecret  string
	catalogText string
	authKey     string
	authKind    string
	profileARN  string
	profileName string
	startURL    string
	region      string
	email       string
	inspector   bundleKiroInspector
	profile     profileentity.Profile
}

func assertKiroBundleRoundTrip(t *testing.T, password string) {
	t.Helper()
	fixture := newKiroBundleFixture()
	path := exportKiroBundleFixture(t, password, fixture)
	importKiroBundleFixture(t, path, password, fixture)
}

func newKiroBundleFixture() kiroBundleFixture {
	fixture := kiroBundleFixture{
		authSecret:  `{"auth_key":"kirocli:social:token","auth_kind":"social","auth_json":"{\\"access_token\\":\\"fixture\\"}","email":"person@example.test","profile_arn":"arn:fixture","profile_name":"main","start_url":"https://example.test/start","region":"us-east-1"}`,
		catalogText: `{"models":[{"id":"model-a","name":"Model A"}]}`,
		authKey:     "kirocli:social:token",
		authKind:    "social",
		profileARN:  "arn:fixture",
		profileName: "main",
		startURL:    "https://example.test/start",
		region:      "us-east-1",
		email:       "person@example.test",
	}
	fixture.inspector = bundleKiroInspector{
		credentials: map[string]profilemodel.BuiltinCredential{
			fixture.authSecret: {
				Provider: profilemodel.ProviderSnapshot{
					Kind: "kiro", AuthKey: &fixture.authKey, AuthKind: &fixture.authKind,
					ProfileARN: &fixture.profileARN, ProfileName: &fixture.profileName,
					StartURL: &fixture.startURL, Region: &fixture.region,
				},
				Email: fixture.email,
			},
		},
		catalogs: map[string]bool{fixture.catalogText: true},
	}
	fixture.profile = profileentity.Profile{
		Name: "kiro", Managed: true, Email: fixture.email,
		Provider: profileentity.Provider{
			Kind: profileentity.ProviderKiro, AuthKey: fixture.authKey, AuthKind: fixture.authKind,
			ProfileARN: fixture.profileARN, ProfileName: fixture.profileName,
			StartURL: fixture.startURL, Region: fixture.region,
		},
	}
	return fixture
}

func exportKiroBundleFixture(t *testing.T, password string, fixture kiroBundleFixture) string {
	t.Helper()
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	catalog.SetKiroInspector(fixture.inspector)
	profile := fixture.profile
	profile.CodexHome = repo.ManagedHome(profile.Name)
	if err := repo.ImportProvider(context.Background(), profile, map[string]string{
		kiroCredentialFile: fixture.authSecret, kiroModelCatalogFile: fixture.catalogText,
	}, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(privateTempDir(t), "kiro-"+boolLabel(password != "")+".json")
	result, err := catalog.Export(context.Background(), profilemodel.ExportRequest{OutputPath: path, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProfileCount != 1 || result.ActiveProfile != "kiro" {
		t.Fatalf("export = %+v", result)
	}
	content, err := repo.ReadBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	payload, encrypted, err := repo.DecodeBundle(content, password)
	if err != nil {
		t.Fatal(err)
	}
	if encrypted != (password != "") || len(payload.Profiles) != 1 {
		t.Fatalf("payload/encrypted = %+v / %t", payload, encrypted)
	}
	assertExportedKiroProfile(t, payload.Profiles[0], fixture)
	return path
}

func assertExportedKiroProfile(t *testing.T, exported profilemodel.ExportedProfile, fixture kiroBundleFixture) {
	t.Helper()
	if exported.AuthJSON != "" || exported.Provider.Kind != "kiro" || len(exported.SecretFiles) != 2 {
		t.Fatalf("exported Kiro profile = %#v", exported)
	}
	if exported.Provider.AuthKey == nil || *exported.Provider.AuthKey != fixture.authKey {
		t.Fatalf("exported auth key = %#v", exported.Provider.AuthKey)
	}
	if exported.Provider.ProfileARN == nil || *exported.Provider.ProfileARN != fixture.profileARN {
		t.Fatalf("exported profile ARN = %#v", exported.Provider.ProfileARN)
	}
}

func importKiroBundleFixture(t *testing.T, path, password string, fixture kiroBundleFixture) {
	t.Helper()
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	catalog.SetKiroInspector(fixture.inspector)
	imported, err := catalog.Import(context.Background(), profilemodel.ImportRequest{Path: path, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	if imported.ImportedCount != 1 || imported.UpdatedCount != 0 || imported.ActiveProfile != "kiro" {
		t.Fatalf("import = %+v", imported)
	}
	stored, err := repo.Resolve(context.Background(), "kiro")
	if err != nil {
		t.Fatal(err)
	}
	wantProvider := fixture.profile.Provider
	if stored.Provider != wantProvider || stored.Email != fixture.email {
		t.Fatalf("stored = %#v", stored)
	}
	assertKiroSecret(t, repo, stored.CodexHome, kiroCredentialFile, fixture.authSecret)
	assertKiroSecret(t, repo, stored.CodexHome, kiroModelCatalogFile, fixture.catalogText)
}

func assertKiroSecret(t *testing.T, repo *profilerepo.Store, home, name, want string) {
	t.Helper()
	got, err := repo.ReadProviderSecret(home, name)
	if err != nil || got != want {
		t.Fatalf("secret %s = %q, err = %v", name, got, err)
	}
}

func TestKiroBundleWithoutCatalogKeepsItOptional(t *testing.T) {
	const authSecret = `{"auth_key":"key","auth_kind":"builder-id","auth_json":"{}"}`
	authKey, authKind := "key", "builder-id"
	inspector := bundleKiroInspector{credentials: map[string]profilemodel.BuiltinCredential{
		authSecret: {Provider: profilemodel.ProviderSnapshot{Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind}},
	}}
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	catalog.SetKiroInspector(inspector)
	profile := profileentity.Profile{Name: "kiro", CodexHome: repo.ManagedHome("kiro"), Managed: true, Provider: profileentity.Provider{Kind: profileentity.ProviderKiro, AuthKey: authKey, AuthKind: authKind}}
	if err := repo.ImportProvider(context.Background(), profile, map[string]string{kiroCredentialFile: authSecret}, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(privateTempDir(t), "kiro.json")
	if _, err := catalog.Export(context.Background(), profilemodel.ExportRequest{OutputPath: path}); err != nil {
		t.Fatal(err)
	}
	content, err := repo.ReadBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	payload, _, err := repo.DecodeBundle(content, "")
	if err != nil || len(payload.Profiles) != 1 || len(payload.Profiles[0].SecretFiles) != 1 || payload.Profiles[0].SecretFiles[0].Path != kiroCredentialFile {
		t.Fatalf("payload = %#v, err = %v", payload, err)
	}
}

func TestKiroBundleUpdateRollbackRestoresAbsentCatalog(t *testing.T) {
	const oldAuth = `{"auth_key":"key","auth_kind":"builder-id","auth_json":"{\"access_token\":\"old\"}"}`
	const newAuth = `{"auth_key":"key","auth_kind":"builder-id","auth_json":"{\"access_token\":\"new\"}"}`
	const catalogText = `{"models":[{"id":"model-a"}]}`
	authKey, authKind := "key", "builder-id"
	inspector := bundleKiroInspector{
		credentials: map[string]profilemodel.BuiltinCredential{
			oldAuth: {Provider: profilemodel.ProviderSnapshot{Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind}},
			newAuth: {Provider: profilemodel.ProviderSnapshot{Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind}},
		},
		catalogs: map[string]bool{catalogText: true},
	}
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	catalog.SetKiroInspector(inspector)
	original := profileentity.Profile{Name: "kiro", CodexHome: repo.ManagedHome("kiro"), Managed: true, Provider: profileentity.Provider{Kind: profileentity.ProviderKiro, AuthKey: authKey, AuthKind: authKind}}
	if err := repo.ImportProvider(context.Background(), original, map[string]string{kiroCredentialFile: oldAuth}, true); err != nil {
		t.Fatal(err)
	}
	occupied := repo.ManagedHome("later")
	if err := os.MkdirAll(occupied, 0o700); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(privateTempDir(t), "rollback.json")
	payload := profilemodel.BundlePayload{
		ExportedAt: "2026-10-01T00:00:00Z", SourceProdexVersion: "0.434.3",
		Profiles: []profilemodel.ExportedProfile{
			{Name: "kiro", Provider: profilemodel.ProviderSnapshot{Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind}, SecretFiles: []profilemodel.ExportedSecretFile{{Path: kiroCredentialFile, Text: newAuth}, {Path: kiroModelCatalogFile, Text: catalogText}}},
			{Name: "later", Provider: profilemodel.ProviderSnapshot{Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind}, SecretFiles: []profilemodel.ExportedSecretFile{{Path: kiroCredentialFile, Text: newAuth}}},
		},
	}
	content, err := repo.EncodeBundle(payload, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.WriteBundle(path, content); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Import(context.Background(), profilemodel.ImportRequest{Path: path}); err == nil {
		t.Fatal("import unexpectedly succeeded despite occupied managed home")
	}
	auth, err := repo.ReadProviderSecret(original.CodexHome, kiroCredentialFile)
	if err != nil || auth != oldAuth {
		t.Fatalf("auth rollback = %q, err = %v", auth, err)
	}
	if _, found, err := repo.ReadOptionalProviderSecret(original.CodexHome, kiroModelCatalogFile); err != nil || found {
		t.Fatalf("catalog rollback found=%t err=%v", found, err)
	}
}

func TestKiroBundleRejectsInvalidSecretSets(t *testing.T) {
	const validAuth = `{"auth_key":"key","auth_kind":"builder-id","auth_json":"{}"}`
	const validCatalog = `{"models":[{"id":"model-a"}]}`
	authKey, authKind := "key", "builder-id"
	inspector := bundleKiroInspector{
		credentials: map[string]profilemodel.BuiltinCredential{validAuth: {Provider: profilemodel.ProviderSnapshot{Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind}}},
		catalogs:    map[string]bool{validCatalog: true},
	}
	catalog := NewCatalog(profilerepo.NewStore(t.TempDir()), &fakeAccounts{}, t.TempDir())
	catalog.SetKiroInspector(inspector)
	for _, files := range [][]profilemodel.ExportedSecretFile{
		nil,
		{{Path: kiroModelCatalogFile, Text: validCatalog}},
		{{Path: "unexpected.json", Text: validAuth}},
		{{Path: kiroCredentialFile, Text: "invalid"}},
		{{Path: kiroCredentialFile, Text: validAuth}, {Path: kiroModelCatalogFile, Text: "invalid"}},
		{{Path: kiroCredentialFile, Text: validAuth}, {Path: kiroCredentialFile, Text: validAuth}},
	} {
		source := profilemodel.ExportedProfile{Name: "kiro", Provider: profilemodel.ProviderSnapshot{Kind: "kiro", AuthKey: &authKey, AuthKind: &authKind}, SecretFiles: files}
		if err := catalog.validateImportedProfile(context.Background(), source); err == nil {
			t.Fatalf("secret files %#v unexpectedly accepted", files)
		}
	}
}
