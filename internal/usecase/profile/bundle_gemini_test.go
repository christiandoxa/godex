package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

func TestGeminiExportMatchesTaggedProviderSerialization(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := profilerepo.NewStore(root)
	profile := profileentity.Profile{
		Name: "gemini", CodexHome: repo.ManagedHome("gemini"), Managed: true,
		Email: "gemini@example.test", Provider: profileentity.Provider{Kind: profileentity.ProviderGemini},
	}
	secret := `{"auth_mode":"oauth","email":"gemini@example.test","access_token":"synthetic-token"}`
	if err := repo.ImportProvider(ctx, profile, map[string]string{geminiOAuthFile: secret}, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(privateTempDir(t), "gemini.json")
	if _, err := NewCatalog(repo, &fakeAccounts{}, "").Export(ctx, profilemodel.ExportRequest{OutputPath: path}); err != nil {
		t.Fatal(err)
	}
	content, err := repo.ReadBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Payload struct {
			Profiles []struct {
				Provider json.RawMessage `json:"provider"`
			} `json:"profiles"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatal(err)
	}
	const taggedProvider = `{"provider_kind":"gemini","email":"gemini@example.test","project_id":null}`
	if len(envelope.Payload.Profiles) != 1 {
		t.Fatalf("Gemini export profile count = %d, want 1", len(envelope.Payload.Profiles))
	}
	var providerJSON bytes.Buffer
	if err := json.Compact(&providerJSON, envelope.Payload.Profiles[0].Provider); err != nil {
		t.Fatal(err)
	}
	if got := providerJSON.String(); got != taggedProvider {
		t.Fatalf("Gemini export provider JSON = %s, want %s", got, taggedProvider)
	}

	openAI, err := json.Marshal(profilemodel.ProviderSnapshot{Kind: "openai"})
	if err != nil || string(openAI) != `{"provider_kind":"openai"}` {
		t.Fatalf("OpenAI provider JSON changed: %s, err=%v", openAI, err)
	}
}

func TestGeminiOAuthBundleRoundTripAndUpdate(t *testing.T) {
	for _, password := range []string{"", "bundle-password"} {
		t.Run(boolLabel(password != ""), func(t *testing.T) {
			assertGeminiOAuthBundleRoundTripAndUpdate(t, password)
		})
	}
}

func assertGeminiOAuthBundleRoundTripAndUpdate(t *testing.T, password string) {
	t.Helper()
	ctx := context.Background()
	const original = `{"auth_mode":"oauth","email":"gemini@example.test","access_token":"synthetic-access-token","expiry_date":1700000000}`
	const replacement = `{"auth_mode":"oauth","email":"gemini@example.test","access_token":"synthetic-next-token"}`
	profile := profileentity.Profile{
		Name: "gemini", CodexHome: filepath.Join(t.TempDir(), "gemini-home"), Managed: true, Email: "gemini@example.test",
		Provider: profileentity.Provider{Kind: profileentity.ProviderGemini, ProjectID: "gemini-project"},
	}
	exportRoot := t.TempDir()
	exportRepo := profilerepo.NewStore(exportRoot)
	profile.CodexHome = exportRepo.ManagedHome(profile.Name)
	if err := exportRepo.ImportProvider(ctx, profile, map[string]string{geminiOAuthFile: original}, true); err != nil {
		t.Fatal(err)
	}
	exportCatalog := NewCatalog(exportRepo, &fakeAccounts{}, "")
	bundlePath := filepath.Join(privateTempDir(t), "gemini.json")
	exportResult, err := exportCatalog.Export(ctx, profilemodel.ExportRequest{OutputPath: bundlePath, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"synthetic-access-token", "synthetic-next-token"} {
		if strings.Contains(exportResult.Path, secret) {
			t.Fatal("export result contains credential material")
		}
	}
	content, err := exportRepo.ReadBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if containsAccessToken := bytes.Contains(content, []byte("synthetic-access-token")); containsAccessToken != (password == "") {
		t.Fatal("bundle credential visibility does not match password protection")
	}
	if containsSecretPath := bytes.Contains(content, []byte(geminiOAuthFile)); containsSecretPath != (password == "") {
		t.Fatal("bundle payload visibility does not match password protection")
	}
	payload, encrypted, err := exportRepo.DecodeBundle(content, password)
	if err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if encrypted != (password != "") || len(payload.Profiles) != 1 {
		t.Fatal("decoded Gemini bundle metadata does not match the requested export")
	}
	exported := payload.Profiles[0]
	if exported.AuthJSON != "" || exported.Provider.Kind != "gemini" || len(exported.SecretFiles) != 1 ||
		exported.Provider.Email == nil || *exported.Provider.Email != "gemini@example.test" ||
		exported.Provider.ProjectID == nil || *exported.Provider.ProjectID != "gemini-project" ||
		exported.SecretFiles[0].Path != geminiOAuthFile || exported.SecretFiles[0].Text != original {
		t.Fatal("exported Gemini profile does not match the expected secret-file payload")
	}
	state, err := os.ReadFile(filepath.Join(exportRoot, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"access_token", "refresh_token", "synthetic-access-token", "synthetic-refresh-token"} {
		if bytes.Contains(state, []byte(secret)) {
			t.Fatal("profile metadata contains credential material")
		}
	}

	importRoot := t.TempDir()
	importRepo := profilerepo.NewStore(importRoot)
	importCatalog := NewCatalog(importRepo, &fakeAccounts{}, "")
	if result, err := importCatalog.Import(ctx, profilemodel.ImportRequest{Path: bundlePath, Password: password}); err != nil ||
		result.ImportedCount != 1 || result.UpdatedCount != 0 || result.ActiveProfile != "gemini" {
		t.Fatalf("import = %+v, err=%v", result, err)
	}
	stored, err := importRepo.Resolve(ctx, "gemini")
	if err != nil || stored.Provider.Kind != profileentity.ProviderGemini ||
		stored.Provider.ProjectID != "gemini-project" || stored.Email != "gemini@example.test" {
		t.Fatalf("stored profile = %#v, err=%v", stored, err)
	}
	secret, err := importRepo.ReadProviderSecret(stored.CodexHome, geminiOAuthFile)
	if err != nil || secret != original {
		t.Fatalf("stored Gemini secret did not match the exported secret file, err=%v", err)
	}
	files, err := os.ReadDir(stored.CodexHome)
	if err != nil || len(files) != 1 || files[0].Name() != geminiOAuthFile {
		t.Fatalf("imported Gemini files = %v, err=%v", files, err)
	}
	state, err = os.ReadFile(filepath.Join(importRoot, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"access_token", "refresh_token", "synthetic-access-token", "synthetic-refresh-token"} {
		if bytes.Contains(state, []byte(secret)) {
			t.Fatal("imported profile metadata contains credential material")
		}
	}

	updatePath := filepath.Join(privateTempDir(t), "gemini-update.json")
	updatedProvider := providerSnapshotFromEntity(profile.Provider)
	updatedEmail := "updated-gemini@example.test"
	updatedProvider.Email = &updatedEmail
	updatedProvider.ProjectID = optionalString("updated-project")
	updatedPayload := profilemodel.BundlePayload{Profiles: []profilemodel.ExportedProfile{{
		Name: "gemini", Email: optionalString("updated-gemini@example.test"), Provider: updatedProvider,
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: replacement}},
	}}}
	encoded, err := importRepo.EncodeBundle(updatedPayload, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := importRepo.WriteBundle(updatePath, encoded); err != nil {
		t.Fatal(err)
	}
	if result, err := importCatalog.Import(ctx, profilemodel.ImportRequest{Path: updatePath}); err != nil ||
		result.UpdatedCount != 1 || result.ActiveProfile != "gemini" {
		t.Fatalf("update = %+v, err=%v", result, err)
	}
	secret, err = importRepo.ReadProviderSecret(stored.CodexHome, geminiOAuthFile)
	if err != nil || secret != replacement {
		t.Fatalf("updated Gemini secret did not match the imported secret file, err=%v", err)
	}
	stored, err = importRepo.Resolve(ctx, "gemini")
	if err != nil || stored.Email != "updated-gemini@example.test" ||
		stored.Provider.Account != "" || stored.Provider.ProjectID != "updated-project" {
		t.Fatalf("updated Gemini profile metadata was not applied, err=%v", err)
	}
	state, err = os.ReadFile(filepath.Join(importRoot, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, credential := range []string{"synthetic-access-token", "synthetic-next-token", "synthetic-refresh-token"} {
		if bytes.Contains(state, []byte(credential)) {
			t.Fatal("updated profile metadata contains credential material")
		}
	}
	invalidUpdate := updatedPayload
	invalidUpdate.Profiles = append([]profilemodel.ExportedProfile(nil), updatedPayload.Profiles...)
	invalidUpdate.Profiles[0].SecretFiles = []profilemodel.ExportedSecretFile{{
		Path: geminiOAuthFile, Text: `{"auth_mode":"oauth","email":"gemini@example.test"}`,
	}}
	encoded, err = importRepo.EncodeBundle(invalidUpdate, "")
	if err != nil {
		t.Fatal(err)
	}
	invalidUpdatePath := filepath.Join(privateTempDir(t), "invalid-update.json")
	if err := importRepo.WriteBundle(invalidUpdatePath, encoded); err != nil {
		t.Fatal(err)
	}
	if _, err := importCatalog.Import(ctx, profilemodel.ImportRequest{Path: invalidUpdatePath}); err == nil {
		t.Fatal("invalid Gemini update unexpectedly imported")
	}
	secret, err = importRepo.ReadProviderSecret(stored.CodexHome, geminiOAuthFile)
	if err != nil || secret != replacement {
		t.Fatalf("invalid update changed the existing Gemini secret, err=%v", err)
	}
}

func TestGeminiBundleImportMatchesTaggedSecretSchema(t *testing.T) {
	ctx := context.Background()
	secret := `{"auth_mode":"","email":"","access_token":"\ud83d\ude00","refresh_token":null,"token_type":null,"scope":null,"expiry_date":null,"project_id":null,"ignored":{"nested":true}}`
	providerEmail := "gemini@example.test"
	repo := profilerepo.NewStore(t.TempDir())
	path := filepath.Join(privateTempDir(t), "empty-fields.json")
	payload := profilemodel.BundlePayload{Profiles: []profilemodel.ExportedProfile{{
		Name: "gemini", Provider: profilemodel.ProviderSnapshot{Kind: "gemini", Email: &providerEmail},
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: secret}},
	}}}
	content, err := repo.EncodeBundle(payload, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.WriteBundle(path, content); err != nil {
		t.Fatal(err)
	}
	catalog := NewCatalog(repo, &fakeAccounts{}, "")
	if _, err := catalog.Import(ctx, profilemodel.ImportRequest{Path: path}); err != nil {
		t.Fatalf("tag-compatible Gemini secret import failed: %v", err)
	}
	stored, err := repo.Resolve(ctx, "gemini")
	if err != nil || stored.Email != providerEmail {
		t.Fatalf("Gemini provider email fallback was not imported, err=%v", err)
	}
}

func TestGeminiBundleImportRejectsInvalidSecretsWithoutCreatingProfiles(t *testing.T) {
	ctx := context.Background()
	valid := `{"auth_mode":"oauth","email":"gemini@example.test","access_token":"synthetic-token"}`
	for _, test := range []struct {
		name     string
		files    []profilemodel.ExportedSecretFile
		provider *profilemodel.ProviderSnapshot
	}{
		{name: "missing secret file"},
		{name: "missing provider email", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: valid}},
			provider: &profilemodel.ProviderSnapshot{Kind: "gemini"}},
		{name: "unexpected secret path", files: []profilemodel.ExportedSecretFile{{Path: "auth.json", Text: valid}}},
		{name: "extra secret file", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: valid}, {Path: "other.json", Text: valid}}},
		{name: "duplicate secret path", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: valid}, {Path: geminiOAuthFile, Text: valid}}},
		{name: "malformed JSON", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: "not-json"}}},
		{name: "lone unicode surrogate", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: `{"auth_mode":"oauth","email":"gemini@example.test","access_token":"synthetic\uD801"}`}}},
		{name: "missing required field", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: `{"auth_mode":"oauth","email":"gemini@example.test"}`}}},
		{name: "null required field", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: `{"auth_mode":"oauth","email":"gemini@example.test","access_token":null}`}}},
		{name: "wrong optional field type", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: `{"auth_mode":"oauth","email":"gemini@example.test","access_token":"synthetic-token","project_id":1}`}}},
		{name: "wrong expiry type", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: `{"auth_mode":"oauth","email":"gemini@example.test","access_token":"synthetic-token","expiry_date":1.5}`}}},
		{name: "duplicate credential field", files: []profilemodel.ExportedSecretFile{{Path: geminiOAuthFile, Text: `{"auth_mode":"oauth","email":"gemini@example.test","access_token":"one","access_token":"two"}`}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := profilerepo.NewStore(t.TempDir())
			provider := profilemodel.ProviderSnapshot{Kind: "gemini", Email: optionalString("gemini@example.test")}
			if test.provider != nil {
				provider = *test.provider
			}
			payload := profilemodel.BundlePayload{Profiles: []profilemodel.ExportedProfile{{
				Name: "gemini", Provider: provider, SecretFiles: test.files,
			}}}
			content, err := repo.EncodeBundle(payload, "")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(privateTempDir(t), "invalid.json")
			if err := repo.WriteBundle(path, content); err != nil {
				t.Fatal(err)
			}
			_, err = NewCatalog(repo, &fakeAccounts{}, "").Import(ctx, profilemodel.ImportRequest{Path: path})
			if err == nil {
				t.Fatal("invalid Gemini bundle unexpectedly imported")
			}
			if strings.Contains(err.Error(), "synthetic-token") {
				t.Fatal("invalid Gemini bundle error contains credential material")
			}
			profiles, err := repo.List(ctx)
			if err != nil || len(profiles) != 0 {
				t.Fatalf("profiles after rejected import = %d, err=%v", len(profiles), err)
			}
		})
	}
}
