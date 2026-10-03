package profile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"
)

func TestGeminiOAuthBundleCommandsRedactCredentialFromSummaries(t *testing.T) {
	ctx := context.Background()
	const secret = `{"auth_mode":"oauth","email":"gemini@example.test","access_token":"synthetic-access-token","refresh_token":"synthetic-refresh-token"}`
	sourceRoot := t.TempDir()
	sourceProfiles := profilerepo.NewStore(sourceRoot)
	sourceProfile := profileentity.Profile{
		Name: "gemini", CodexHome: sourceProfiles.ManagedHome("gemini"), Managed: true,
		Email: "gemini@example.test", Provider: profileentity.Provider{Kind: profileentity.ProviderGemini},
	}
	if err := sourceProfiles.ImportProvider(ctx, sourceProfile, map[string]string{"gemini_oauth.json": secret}, true); err != nil {
		t.Fatal(err)
	}
	sourceCatalog := profileusecase.NewCatalog(sourceProfiles, accountrepo.NewFileStore(sourceRoot), "")
	bundleDir := t.TempDir()
	if err := os.Chmod(bundleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(bundleDir, "gemini.json")
	t.Setenv(exportPasswordEnv, "")
	var exportOutput bytes.Buffer
	if err := Run(ctx, sourceCatalog, &exportOutput, []string{"export", "--no-password", bundlePath}); err != nil {
		t.Fatal(err)
	}

	importRoot := t.TempDir()
	importCatalog := profileusecase.NewCatalog(
		profilerepo.NewStore(importRoot), accountrepo.NewFileStore(importRoot), "",
	)
	t.Setenv(importPasswordEnv, "")
	var importOutput bytes.Buffer
	if err := Run(ctx, importCatalog, &importOutput, []string{"import", bundlePath}); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{exportOutput.String(), importOutput.String()} {
		if strings.Contains(output, "synthetic-access-token") || strings.Contains(output, "synthetic-refresh-token") {
			t.Fatal("profile bundle command summary contains credential material")
		}
	}
	if _, err := os.Stat(filepath.Join(importRoot, "profiles", "gemini", "gemini_oauth.json")); err != nil {
		t.Fatalf("imported Gemini secret file is missing: %v", err)
	}
}
