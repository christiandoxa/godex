package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadResolvesConfiguredHomeAndCodexBinary(t *testing.T) {
	t.Setenv(HomeEnv, filepath.Join("relative", "godex"))
	t.Setenv(CodexBinEnv, "/opt/codex")
	t.Setenv(AgyBinEnv, "/opt/agy")
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
	if got.Home != wantHome || got.CodexBin != "/opt/codex" || got.AgyBin != "/opt/agy" || got.UpstreamURL != "http://127.0.0.1:9999/backend-api" || got.CurrentCodexHome != wantCodexHome {
		t.Fatalf("config = %#v, want home %q and codex %q", got, wantHome, "/opt/codex")
	}
}

func TestLoadUsesAbsoluteDefaultHomeAndCodexBinary(t *testing.T) {
	t.Setenv(HomeEnv, "")
	t.Setenv(CodexBinEnv, "")
	t.Setenv(AgyBinEnv, "")
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
	if got.Home != wantHome || got.CodexBin != "codex" || got.AgyBin != "agy" || got.CurrentCodexHome != wantCodexHome {
		t.Fatalf("config = %#v, want home %q and codex %q", got, wantHome, "codex")
	}
}

func TestLoadResolvesProdexSharedCodexHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv(ProdexHomeEnv, root)
	t.Setenv(ProdexSharedCodexHomeEnv, filepath.Join("shared", "codex"))
	t.Setenv(CodexHomeEnv, filepath.Join(t.TempDir(), "ambient"))

	got, err := LoadAntigravity()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "shared", "codex")
	if got.SharedCodexHome != want {
		t.Fatalf("shared Codex home = %q, want %q", got.SharedCodexHome, want)
	}

	absolute := filepath.Join(t.TempDir(), "shared-codex")
	t.Setenv(ProdexSharedCodexHomeEnv, absolute)
	got, err = LoadAntigravity()
	if err != nil {
		t.Fatal(err)
	}
	if got.SharedCodexHome != absolute {
		t.Fatalf("absolute shared Codex home = %q, want %q", got.SharedCodexHome, absolute)
	}
}

func TestLoadResolvesSharedCodexHomeForNativeAuthentication(t *testing.T) {
	root := t.TempDir()
	t.Setenv(HomeEnv, filepath.Join(t.TempDir(), "godex"))
	t.Setenv(ProdexHomeEnv, root)
	t.Setenv(ProdexSharedCodexHomeEnv, filepath.Join("shared", "codex"))
	t.Setenv(CodexHomeEnv, t.TempDir())

	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "shared", "codex"); settings.SharedCodexHome != want {
		t.Fatalf("shared Codex home = %q, want %q", settings.SharedCodexHome, want)
	}
}

func TestLoadAntigravityDefaultsToSharedCodexDirectory(t *testing.T) {
	original, found := os.LookupEnv(ProdexSharedCodexHomeEnv)
	if err := os.Unsetenv(ProdexSharedCodexHomeEnv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if found {
			_ = os.Setenv(ProdexSharedCodexHomeEnv, original)
		} else {
			_ = os.Unsetenv(ProdexSharedCodexHomeEnv)
		}
	})

	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := LoadAntigravity()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(userHome, ".codex"); settings.SharedCodexHome != want {
		t.Fatalf("default shared Codex home = %q, want %q", settings.SharedCodexHome, want)
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
