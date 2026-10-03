package profile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

const bundleTestEmail = "person@example.test"

type bundleInspector struct {
	identities map[string]accountentity.Identity
}

func (inspector bundleInspector) InspectAuthJSON(_ context.Context, content []byte) (accountentity.Identity, error) {
	identity, ok := inspector.identities[string(content)]
	if !ok {
		return accountentity.Identity{}, errors.New("invalid auth")
	}
	return identity, nil
}

func TestProfileBundleWorkflowPlainAndEncrypted(t *testing.T) {
	for _, password := range []string{"", "bundle-password"} {
		name := boolLabel(password != "")
		t.Run(name, func(t *testing.T) {
			assertProfileBundleWorkflow(t, password)
		})
	}
}

func assertProfileBundleWorkflow(t *testing.T, password string) {
	t.Helper()
	exportRepo := profilerepo.NewStore(t.TempDir())
	exportCatalog := NewCatalog(exportRepo, &fakeAccounts{}, t.TempDir())
	inspector := bundleInspector{identities: map[string]accountentity.Identity{
		"auth-work": {Email: bundleTestEmail, ChatGPTAccountID: "workspace-1"},
	}}
	exportCatalog.SetAuthInspector(inspector)
	added, err := exportCatalog.Add(context.Background(), profilemodel.AddRequest{Name: "work"})
	if err != nil {
		t.Fatal(err)
	}
	writeAuthFixture(t, added.Profile.CodexHome, "auth-work")
	bundleDir := privateTempDir(t)
	path := filepath.Join(bundleDir, "profiles-"+boolLabel(password != "")+".json")
	result, err := exportCatalog.Export(context.Background(), profilemodel.ExportRequest{OutputPath: path, Password: password})
	if err != nil || result.ProfileCount != 1 || result.Encrypted != (password != "") || result.ActiveProfile != "work" {
		t.Fatalf("export = %+v, err = %v", result, err)
	}

	importRepo := profilerepo.NewStore(t.TempDir())
	importCatalog := NewCatalog(importRepo, &fakeAccounts{}, t.TempDir())
	importCatalog.SetAuthInspector(inspector)
	imported, err := importCatalog.Import(context.Background(), profilemodel.ImportRequest{Path: path, Password: password})
	if err != nil || imported.ImportedCount != 1 || imported.UpdatedCount != 0 || imported.ActiveProfile != "work" {
		t.Fatalf("import = %+v, err = %v", imported, err)
	}
	current, err := importCatalog.Current(context.Background())
	if err != nil || current.Profile.Name != "work" {
		t.Fatalf("current = %+v, err = %v", current, err)
	}
	auth, err := importRepo.ReadAuthJSON(current.Profile.CodexHome)
	if err != nil || string(auth) != "auth-work" {
		t.Fatalf("imported auth = %q, err = %v", auth, err)
	}
	files, err := os.ReadDir(current.Profile.CodexHome)
	if err != nil || len(files) != 1 || files[0].Name() != "auth.json" {
		t.Fatalf("committed import lifecycle marker remains: files=%v err=%v", files, err)
	}
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func TestProfileBundleImportUpdatesOnlyVerifiedIdentity(t *testing.T) {
	root := t.TempDir()
	repo := profilerepo.NewStore(root)
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	inspector := bundleInspector{identities: map[string]accountentity.Identity{
		"auth-old":   {Email: bundleTestEmail, ChatGPTAccountID: "workspace-1"},
		"auth-new":   {Email: bundleTestEmail, ChatGPTAccountID: "workspace-1"},
		"auth-other": {Email: "other@example.test", ChatGPTAccountID: "workspace-2"},
	}}
	catalog.SetAuthInspector(inspector)
	added, err := catalog.Add(context.Background(), profilemodel.AddRequest{Name: "work"})
	if err != nil {
		t.Fatal(err)
	}
	writeAuthFixture(t, added.Profile.CodexHome, "auth-old")

	bundleDir := privateTempDir(t)
	path := filepath.Join(bundleDir, "update.json")
	writeBundleFixture(t, repo, path, "work", "auth-new")
	result, err := catalog.Import(context.Background(), profilemodel.ImportRequest{Path: path})
	if err != nil || result.UpdatedCount != 1 || result.ImportedCount != 0 {
		t.Fatalf("verified update = %+v, err = %v", result, err)
	}
	auth, err := repo.ReadAuthJSON(added.Profile.CodexHome)
	if err != nil || string(auth) != "auth-new" {
		t.Fatalf("updated auth = %q, err = %v", auth, err)
	}
	if journals, err := repo.BundleImportJournals(); err != nil || len(journals) != 0 {
		t.Fatalf("committed import journals = %+v, err=%v", journals, err)
	}
	files, err := os.ReadDir(added.Profile.CodexHome)
	if err != nil || len(files) != 1 || files[0].Name() != "auth.json" {
		t.Fatalf("committed import backups remain: files=%v err=%v", files, err)
	}

	writeBundleFixture(t, repo, path, "work", "auth-other")
	if _, err := catalog.Import(context.Background(), profilemodel.ImportRequest{Path: path}); err == nil {
		t.Fatal("identity mismatch unexpectedly updated existing profile")
	}
	auth, err = repo.ReadAuthJSON(added.Profile.CodexHome)
	if err != nil || string(auth) != "auth-new" {
		t.Fatalf("mismatch changed auth = %q, err = %v", auth, err)
	}
}

func TestProfileBundleImportRejectsDuplicateResolvedTargets(t *testing.T) {
	catalog := NewCatalog(profilerepo.NewStore(t.TempDir()), &fakeAccounts{}, t.TempDir())
	catalog.SetAuthInspector(bundleInspector{identities: map[string]accountentity.Identity{
		"auth-one": {Email: bundleTestEmail, ChatGPTAccountID: "workspace-1"},
		"auth-two": {Email: bundleTestEmail, ChatGPTAccountID: "workspace-1"},
	}})
	_, err := catalog.planImport(context.Background(), profilemodel.BundlePayload{Profiles: []profilemodel.ExportedProfile{
		{Name: "first", Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: "auth-one"},
		{Name: "second", Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: "auth-two"},
	}})
	if err == nil || err.Error() != `profile export entries resolve to duplicate target "first"` {
		t.Fatalf("duplicate target error = %v", err)
	}
}

func writeAuthFixture(t *testing.T, home, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeBundleFixture(t *testing.T, repo *profilerepo.Store, path, name, auth string) {
	t.Helper()
	payload := profilemodel.BundlePayload{
		ExportedAt: "2026-10-01T00:00:00Z", SourceProdexVersion: "0.434.2",
		Profiles: []profilemodel.ExportedProfile{{
			Name: name, SourceManaged: true, Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: auth,
			SecretFiles: []profilemodel.ExportedSecretFile{},
		}},
	}
	content, err := repo.EncodeBundle(payload, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.WriteBundle(path, content); err != nil {
		t.Fatal(err)
	}
}

func boolLabel(value bool) string {
	if value {
		return "encrypted"
	}
	return "plain"
}

type bundleClaudeInspector struct {
	credentials map[string]profilemodel.BuiltinCredential
}

func (inspector bundleClaudeInspector) Load(context.Context) (profilemodel.BuiltinCredential, error) {
	return profilemodel.BuiltinCredential{}, errors.New("external Claude source should not be used by bundle tests")
}

func (inspector bundleClaudeInspector) InspectCredential(_ context.Context, text string) (profilemodel.BuiltinCredential, error) {
	credential, ok := inspector.credentials[text]
	if !ok {
		return profilemodel.BuiltinCredential{}, errors.New("invalid Claude credential fixture")
	}
	return credential, nil
}

func TestAnthropicBundleRoundTripPlainAndEncrypted(t *testing.T) {
	for _, password := range []string{"", "bundle-password"} {
		t.Run(boolLabel(password != ""), func(t *testing.T) {
			assertAnthropicBundleRoundTrip(t, password)
		})
	}
}

func assertAnthropicBundleRoundTrip(t *testing.T, password string) {
	t.Helper()
	const secret = `{"claudeAiOauth":{"accessToken":"fixture-access","subscriptionType":"pro","email":"person@example.test"}}`
	account := "person@example.test"
	method := "claude-ai-oauth:pro"
	inspector := bundleClaudeInspector{credentials: map[string]profilemodel.BuiltinCredential{
		secret: {
			Provider:    profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &account, AuthMethod: &method},
			Email:       account,
			SecretFiles: []profilemodel.ExportedSecretFile{{Path: claudeCredentialFile, Text: secret}},
		},
	}}

	exportRepo := profilerepo.NewStore(t.TempDir())
	exportCatalog := NewCatalog(exportRepo, &fakeAccounts{}, t.TempDir())
	exportCatalog.SetClaudeSource(inspector)
	profile := profileentity.Profile{
		Name: "claude", CodexHome: exportRepo.ManagedHome("claude"), Managed: true, Email: account,
		Provider: profileentity.Provider{Kind: profileentity.ProviderAnthropic, Account: account, AuthMethod: method},
	}
	if err := exportRepo.ImportProvider(context.Background(), profile, map[string]string{claudeCredentialFile: secret}, true); err != nil {
		t.Fatal(err)
	}
	bundleDir := privateTempDir(t)
	path := filepath.Join(bundleDir, "anthropic-"+boolLabel(password != "")+".json")
	result, err := exportCatalog.Export(context.Background(), profilemodel.ExportRequest{OutputPath: path, Password: password})
	if err != nil || result.ProfileCount != 1 || result.ActiveProfile != "claude" {
		t.Fatalf("export = %+v, err = %v", result, err)
	}
	content, err := exportRepo.ReadBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	payload, encrypted, err := exportRepo.DecodeBundle(content, password)
	if err != nil || encrypted != (password != "") || len(payload.Profiles) != 1 {
		t.Fatalf("payload/encrypted/err = %+v / %t / %v", payload, encrypted, err)
	}
	exported := payload.Profiles[0]
	if exported.AuthJSON != "" || exported.Provider.Kind != "anthropic" || len(exported.SecretFiles) != 1 || exported.SecretFiles[0].Path != claudeCredentialFile || exported.SecretFiles[0].Text != secret {
		t.Fatalf("exported Anthropic profile = %#v", exported)
	}

	importRepo := profilerepo.NewStore(t.TempDir())
	importCatalog := NewCatalog(importRepo, &fakeAccounts{}, t.TempDir())
	importCatalog.SetClaudeSource(inspector)
	imported, err := importCatalog.Import(context.Background(), profilemodel.ImportRequest{Path: path, Password: password})
	if err != nil || imported.ImportedCount != 1 || imported.UpdatedCount != 0 || imported.ActiveProfile != "claude" {
		t.Fatalf("import = %+v, err = %v", imported, err)
	}
	stored, err := importRepo.Resolve(context.Background(), "claude")
	if err != nil || stored.Provider.Kind != profileentity.ProviderAnthropic || stored.Provider.Account != account || stored.Provider.AuthMethod != method || stored.Email != account {
		t.Fatalf("stored = %#v, err = %v", stored, err)
	}
	storedSecret, err := importRepo.ReadProviderSecret(stored.CodexHome, claudeCredentialFile)
	if err != nil || storedSecret != secret {
		t.Fatalf("stored secret = %q, err = %v", storedSecret, err)
	}
}

func TestAnthropicBundleUpdateRollsBackWhenLaterProfileFails(t *testing.T) {
	const oldSecret = `{"accessToken":"old","email":"person@example.test"}`
	const newSecret = `{"accessToken":"new","email":"person@example.test"}`
	const newProfileSecret = `{"accessToken":"new-profile","email":"new@example.test"}`
	account := "person@example.test"
	newAccount := "new@example.test"
	method := "claude-ai-oauth"
	inspector := bundleClaudeInspector{credentials: map[string]profilemodel.BuiltinCredential{
		oldSecret:        {Provider: profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &account, AuthMethod: &method}, Email: account},
		newSecret:        {Provider: profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &account, AuthMethod: &method}, Email: account},
		newProfileSecret: {Provider: profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &newAccount, AuthMethod: &method}, Email: newAccount},
	}}
	repo := &failingBundleImportRepository{Store: profilerepo.NewStore(t.TempDir()), failName: "new-profile"}
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	catalog.SetClaudeSource(inspector)
	original := profileentity.Profile{
		Name: "claude", CodexHome: repo.ManagedHome("claude"), Managed: true, Email: account,
		Provider: profileentity.Provider{Kind: profileentity.ProviderAnthropic, Account: account, AuthMethod: method},
	}
	if err := repo.ImportProvider(context.Background(), original, map[string]string{claudeCredentialFile: oldSecret}, true); err != nil {
		t.Fatal(err)
	}
	bundleDir := privateTempDir(t)
	path := filepath.Join(bundleDir, "rollback.json")
	payload := profilemodel.BundlePayload{
		ExportedAt: "2026-10-01T00:00:00Z", SourceProdexVersion: "0.434.3",
		Profiles: []profilemodel.ExportedProfile{
			{
				Name: "claude", Email: &account, SourceManaged: true,
				Provider:    profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &account, AuthMethod: &method},
				SecretFiles: []profilemodel.ExportedSecretFile{{Path: claudeCredentialFile, Text: newSecret}},
			},
			{
				Name: "new-profile", Email: &newAccount, SourceManaged: true,
				Provider:    profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &newAccount, AuthMethod: &method},
				SecretFiles: []profilemodel.ExportedSecretFile{{Path: claudeCredentialFile, Text: newProfileSecret}},
			},
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
	secret, err := repo.ReadProviderSecret(original.CodexHome, claudeCredentialFile)
	if err != nil || secret != oldSecret {
		t.Fatalf("rollback secret = %q, err = %v", secret, err)
	}
	stored, err := repo.Resolve(context.Background(), "claude")
	if err != nil || stored.Email != original.Email || stored.Provider != original.Provider {
		t.Fatalf("rollback profile = %#v, err = %v", stored, err)
	}
}

func TestAnthropicBundleRejectsMissingUnexpectedOrInvalidSecret(t *testing.T) {
	account := "person@example.test"
	method := "claude-ai-oauth"
	valid := `{"accessToken":"valid"}`
	inspector := bundleClaudeInspector{credentials: map[string]profilemodel.BuiltinCredential{
		valid: {Provider: profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &account, AuthMethod: &method}, Email: account},
	}}
	catalog := NewCatalog(profilerepo.NewStore(t.TempDir()), &fakeAccounts{}, t.TempDir())
	catalog.SetClaudeSource(inspector)
	for _, files := range [][]profilemodel.ExportedSecretFile{
		nil,
		{{Path: "unexpected.json", Text: valid}},
		{{Path: claudeCredentialFile, Text: "invalid"}},
		{{Path: claudeCredentialFile, Text: valid}, {Path: claudeCredentialFile, Text: valid}},
	} {
		source := profilemodel.ExportedProfile{
			Name: "claude", Provider: profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &account, AuthMethod: &method}, SecretFiles: files,
		}
		if err := catalog.validateImportedProfile(context.Background(), source); err == nil {
			t.Fatalf("secret files %#v unexpectedly accepted", files)
		}
	}
}

type failingBundleImportRepository struct {
	*profilerepo.Store
	failName string
}

func (repo *failingBundleImportRepository) ImportBundleProfile(
	ctx context.Context,
	value profileentity.Profile,
	files map[string][]byte,
	id string,
) error {
	if value.Name == repo.failName {
		return errors.New("injected provider import failure")
	}
	return repo.Store.ImportBundleProfile(ctx, value, files, id)
}
