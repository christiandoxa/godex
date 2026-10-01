package profile

import (
	"context"
	"path/filepath"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

func TestCopilotBundleRoundTripPlainAndEncrypted(t *testing.T) {
	for _, password := range []string{"", "bundle-password"} {
		t.Run(boolLabel(password != ""), func(t *testing.T) {
			assertCopilotBundleRoundTrip(t, password)
		})
	}
}

func assertCopilotBundleRoundTrip(t *testing.T, password string) {
	t.Helper()
	provider := profileentity.Provider{
		Kind: profileentity.ProviderCopilot,
		Host: "https://github.example.test", Login: "octocat",
		APIURL: "https://copilot-api.example.test", AccessTypeSKU: "enterprise",
		CopilotPlan: "business",
	}
	exportRepo := profilerepo.NewStore(t.TempDir())
	exportCatalog := NewCatalog(exportRepo, &fakeAccounts{}, t.TempDir())
	profile := profileentity.Profile{
		Name: "copilot", CodexHome: exportRepo.ManagedHome("copilot"), Managed: true,
		Email: "octocat@example.test", Provider: provider,
	}
	if err := exportRepo.ImportProvider(context.Background(), profile, map[string]string{}, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(privateTempDir(t), "copilot-"+boolLabel(password != "")+".json")
	result, err := exportCatalog.Export(context.Background(), profilemodel.ExportRequest{OutputPath: path, Password: password})
	if err != nil || result.ProfileCount != 1 || result.ActiveProfile != "copilot" {
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
	assertCopilotExportedProfile(t, exported, provider)

	importRepo := profilerepo.NewStore(t.TempDir())
	importCatalog := NewCatalog(importRepo, &fakeAccounts{}, t.TempDir())
	imported, err := importCatalog.Import(context.Background(), profilemodel.ImportRequest{Path: path, Password: password})
	if err != nil || imported.ImportedCount != 1 || imported.UpdatedCount != 0 || imported.ActiveProfile != "copilot" {
		t.Fatalf("import = %+v, err = %v", imported, err)
	}
	stored, err := importRepo.Resolve(context.Background(), "copilot")
	if err != nil || stored.Provider != provider || stored.Email != profile.Email {
		t.Fatalf("stored = %#v, err = %v", stored, err)
	}
}

func assertCopilotExportedProfile(t *testing.T, exported profilemodel.ExportedProfile, provider profileentity.Provider) {
	t.Helper()
	if exported.AuthJSON != "" || len(exported.SecretFiles) != 0 || exported.Provider.Kind != "copilot" {
		t.Fatalf("exported Copilot profile = %#v", exported)
	}
	checks := map[string]struct {
		value *string
		want  string
	}{
		"host":            {exported.Provider.Host, provider.Host},
		"login":           {exported.Provider.Login, provider.Login},
		"api_url":         {exported.Provider.APIURL, provider.APIURL},
		"access_type_sku": {exported.Provider.AccessTypeSKU, provider.AccessTypeSKU},
		"copilot_plan":    {exported.Provider.CopilotPlan, provider.CopilotPlan},
	}
	for name, check := range checks {
		if check.value == nil || *check.value != check.want {
			t.Fatalf("%s = %#v, want %q", name, check.value, check.want)
		}
	}
}

func TestCopilotBundleUpdatesExistingProfileMetadata(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	oldProvider := profileentity.Provider{
		Kind: profileentity.ProviderCopilot, Host: "github.com", Login: "old-login",
		APIURL: "https://api.githubcopilot.com", CopilotPlan: "individual",
	}
	profile := profileentity.Profile{
		Name: "copilot", CodexHome: repo.ManagedHome("copilot"), Managed: true,
		Email: "old@example.test", Provider: oldProvider,
	}
	if err := repo.ImportProvider(context.Background(), profile, map[string]string{}, true); err != nil {
		t.Fatal(err)
	}
	newHost, newLogin := "https://github.example.test", "new-login"
	newAPI, sku, plan := "https://copilot-api.example.test", "enterprise", "business"
	newEmail := "new@example.test"
	payload := profilemodel.BundlePayload{
		ExportedAt: "2026-10-01T00:00:00Z", SourceProdexVersion: "0.434.3",
		Profiles: []profilemodel.ExportedProfile{{
			Name: "copilot", Email: &newEmail, SourceManaged: true,
			Provider: profilemodel.ProviderSnapshot{
				Kind: "copilot", Host: &newHost, Login: &newLogin, APIURL: &newAPI,
				AccessTypeSKU: &sku, CopilotPlan: &plan,
			},
			AuthJSON: "ignored-for-copilot", SecretFiles: []profilemodel.ExportedSecretFile{},
		}},
	}
	path := filepath.Join(privateTempDir(t), "copilot-update.json")
	content, err := repo.EncodeBundle(payload, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.WriteBundle(path, content); err != nil {
		t.Fatal(err)
	}
	result, err := catalog.Import(context.Background(), profilemodel.ImportRequest{Path: path})
	if err != nil || result.ImportedCount != 0 || result.UpdatedCount != 1 {
		t.Fatalf("update = %+v, err = %v", result, err)
	}
	stored, err := repo.Resolve(context.Background(), "copilot")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Email != newEmail || stored.Provider.Host != newHost || stored.Provider.Login != newLogin || stored.Provider.APIURL != newAPI || stored.Provider.AccessTypeSKU != sku || stored.Provider.CopilotPlan != plan {
		t.Fatalf("updated profile = %#v", stored)
	}
}
