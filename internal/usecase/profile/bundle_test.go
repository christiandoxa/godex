package profile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
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

	writeBundleFixture(t, repo, path, "work", "auth-other")
	if _, err := catalog.Import(context.Background(), profilemodel.ImportRequest{Path: path}); err == nil {
		t.Fatal("identity mismatch unexpectedly updated existing profile")
	}
	auth, err = repo.ReadAuthJSON(added.Profile.CodexHome)
	if err != nil || string(auth) != "auth-new" {
		t.Fatalf("mismatch changed auth = %q, err = %v", auth, err)
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
