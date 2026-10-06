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

func TestProdex04355AgyBundleRoundTripMetadataOnly(t *testing.T) {
	for _, password := range []string{"", "bundle-password"} {
		t.Run(boolLabel(password != ""), func(t *testing.T) {
			provider := profileentity.Provider{Kind: profileentity.ProviderAgy, Account: "agy@example.test"}
			exportRepo := profilerepo.NewStore(t.TempDir())
			exportCatalog := NewCatalog(exportRepo, &fakeAccounts{}, t.TempDir())
			profile := profileentity.Profile{
				Name: "agy-main", CodexHome: exportRepo.ManagedHome("agy-main"), Managed: true,
				Email: "agy@example.test", Provider: provider,
			}
			if err := exportRepo.ImportProvider(context.Background(), profile, map[string]string{}, true); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(privateTempDir(t), "agy-"+boolLabel(password != "")+".json")
			result, err := exportCatalog.Export(context.Background(), profilemodel.ExportRequest{OutputPath: path, Password: password})
			if err != nil || result.ProfileCount != 1 || result.ActiveProfile != "agy-main" {
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
			if exported.Provider.Kind != "agy" || exported.Provider.Account == nil || *exported.Provider.Account != provider.Account ||
				exported.AuthJSON != "" || len(exported.SecretFiles) != 0 {
				t.Fatalf("exported Agy profile = %#v", exported)
			}

			importRepo := profilerepo.NewStore(t.TempDir())
			importCatalog := NewCatalog(importRepo, &fakeAccounts{}, t.TempDir())
			imported, err := importCatalog.Import(context.Background(), profilemodel.ImportRequest{Path: path, Password: password})
			if err != nil || imported.ImportedCount != 1 || imported.UpdatedCount != 0 || imported.ActiveProfile != "agy-main" {
				t.Fatalf("import = %+v, err = %v", imported, err)
			}
			stored, err := importRepo.Resolve(context.Background(), "agy-main")
			if err != nil || stored.Provider != provider || stored.Email != profile.Email {
				t.Fatalf("stored = %#v, err = %v", stored, err)
			}
		})
	}
}

func TestProdex04355AgyBundleRejectsUnexpectedSecretFiles(t *testing.T) {
	catalog := NewCatalog(profilerepo.NewStore(t.TempDir()), &fakeAccounts{}, t.TempDir())
	account := "agy@example.test"
	err := catalog.validateImportedProfile(context.Background(), profilemodel.ExportedProfile{
		Name: "agy-main", Provider: profilemodel.ProviderSnapshot{Kind: "agy", Account: &account},
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: "unexpected.json", Text: "secret"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected provider secret files") {
		t.Fatalf("unexpected Agy secret validation error = %v", err)
	}
}
