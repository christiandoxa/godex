package kiro

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func TestKiroProfileQuotaMatchesImportedSnapshotContract(t *testing.T) {
	home := t.TempDir()
	secret := `{"auth_key":"key","auth_kind":"builder-id","auth_json":"{}","email":"person@example.test","profile_name":"main","region":"us-east-1"}`
	catalog := `{"models":[{"id":"model-a"},{"modelId":"model-b"}]}`
	if err := os.WriteFile(filepath.Join(home, CredentialsFile), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ModelCatalogFile), []byte(catalog), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := NewSource().FetchQuota(context.Background(), profilemodel.QuotaTarget{CodexHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if info.Provider != "Kiro CLI" || info.Account != "person@example.test" || info.Plan != "builder-id" || info.Status != "Ready (imported)" || info.Main != "2 imported models" || info.Available == nil || !*info.Available {
		t.Fatalf("info = %#v", info)
	}
	want := map[string]string{"Auth method": "builder-id", "Profile": "main", "Region": "us-east-1", "Models": "2"}
	if len(info.Details) != len(want) {
		t.Fatalf("details = %#v", info.Details)
	}
	for _, detail := range info.Details {
		if want[detail.Label] != detail.Value {
			t.Fatalf("detail = %#v, all = %#v", detail, info.Details)
		}
	}
}

func TestKiroProfileQuotaAllowsMissingCatalog(t *testing.T) {
	home := t.TempDir()
	secret := `{"auth_key":"key","auth_kind":"social","auth_json":"{}","profile_arn":"arn:fixture"}`
	if err := os.WriteFile(filepath.Join(home, CredentialsFile), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := NewSource().FetchQuota(context.Background(), profilemodel.QuotaTarget{CodexHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if info.Account != "arn:fixture" || info.Main != "credential snapshot available" || info.Details[len(info.Details)-1].Value != "catalog unavailable" {
		t.Fatalf("info = %#v", info)
	}
}

func TestKiroProfileQuotaRejectsInvalidOrSymlinkedSnapshots(t *testing.T) {
	home := t.TempDir()
	if _, err := NewSource().FetchQuota(context.Background(), profilemodel.QuotaTarget{CodexHome: home}); err == nil {
		t.Fatal("missing Kiro auth snapshot unexpectedly accepted")
	}
	if err := os.WriteFile(filepath.Join(home, CredentialsFile), []byte(`{"auth_key":"key","auth_kind":"social","auth_json":"{}"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ModelCatalogFile), []byte(`{"models":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSource().FetchQuota(context.Background(), profilemodel.QuotaTarget{CodexHome: home}); err == nil || !strings.Contains(err.Error(), "model catalog") {
		t.Fatalf("invalid catalog error = %v", err)
	}
	if runtime.GOOS != "windows" {
		other := filepath.Join(t.TempDir(), "auth.json")
		if err := os.WriteFile(other, []byte(`{"auth_key":"key","auth_kind":"social","auth_json":"{}"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(home, CredentialsFile)); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(other, filepath.Join(home, CredentialsFile)); err != nil {
			t.Fatal(err)
		}
		if _, err := NewSource().FetchQuota(context.Background(), profilemodel.QuotaTarget{CodexHome: home}); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("symlink auth error = %v", err)
		}
	}
}
