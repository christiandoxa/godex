package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadResolvesConfiguredHomeAndCodexBinary(t *testing.T) {
	t.Setenv(HomeEnv, filepath.Join("relative", "godex"))
	t.Setenv(CodexBinEnv, "/opt/codex")
	t.Setenv(CodexHomeEnv, filepath.Join("relative", "codex"))
	t.Setenv(UpstreamEnv, "http://127.0.0.1:9999/backend-api")

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	wantHome, err := filepath.Abs(filepath.Join("relative", "godex"))
	if err != nil {
		t.Fatal(err)
	}
	wantCodexHome, err := filepath.Abs(filepath.Join("relative", "codex"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Home != wantHome || got.CodexBin != "/opt/codex" || got.UpstreamURL != "http://127.0.0.1:9999/backend-api" || got.CurrentCodexHome != wantCodexHome {
		t.Fatalf("config = %#v, want home %q and codex %q", got, wantHome, "/opt/codex")
	}
}

func TestLoadUsesAbsoluteDefaultHomeAndCodexBinary(t *testing.T) {
	t.Setenv(HomeEnv, "")
	t.Setenv(CodexBinEnv, "")
	t.Setenv(CodexHomeEnv, "")

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	wantHome, err := filepath.Abs(filepath.Join(userHome, ".godex"))
	if err != nil {
		t.Fatal(err)
	}
	wantCodexHome, err := filepath.Abs(filepath.Join(userHome, ".codex"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Home != wantHome || got.CodexBin != "codex" || got.CurrentCodexHome != wantCodexHome {
		t.Fatalf("config = %#v, want home %q and codex %q", got, wantHome, "codex")
	}
}

func TestLoadRejectsFilesystemRoot(t *testing.T) {
	t.Setenv(HomeEnv, string(filepath.Separator))
	if _, err := Load(); err == nil {
		t.Fatal("filesystem root unexpectedly accepted as GODEX_HOME")
	}
	t.Setenv(HomeEnv, filepath.Join(t.TempDir(), "godex"))
	t.Setenv(CodexHomeEnv, string(filepath.Separator))
	if _, err := Load(); err == nil {
		t.Fatal("filesystem root unexpectedly accepted as CODEX_HOME")
	}
}
