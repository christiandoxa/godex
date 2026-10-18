package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProdex04382OracleIsolatesNativeHomeAndCodexExecutable(t *testing.T) {
	t.Setenv("USERPROFILE", "synthetic-ambient-home")
	t.Setenv("APPDATA", "synthetic-ambient-appdata")
	t.Setenv("LOCALAPPDATA", "synthetic-ambient-localappdata")
	t.Setenv("PRODEX_SHARED_CODEX_HOME", "synthetic-ambient-shared")
	t.Setenv("GEMINI_API_KEY", "synthetic-ambient-key")
	p := product{user: filepath.Join(t.TempDir(), "user"), state: filepath.Join(t.TempDir(), "state")}
	values := make(map[string]string)
	environment, err := productEnv(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		values[strings.ToUpper(key)] = value
	}
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if values[key] != p.user {
			t.Fatalf("%s escaped synthetic home: %q", key, values[key])
		}
	}
	for _, key := range []string{"APPDATA", "LOCALAPPDATA"} {
		if !strings.HasPrefix(values[key], p.user+string(filepath.Separator)) {
			t.Fatalf("%s escaped isolated home", key)
		}
	}
	if values["PRODEX_SHARED_CODEX_HOME"] != filepath.Join(p.user, ".codex") || values["GEMINI_API_KEY"] != "" {
		t.Fatal("ambient shared home or provider credentials escaped isolation")
	}
	want, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if values["PRODEX_CODEX_BIN"] != want || values["GODEX_CODEX_BIN"] != want {
		t.Fatal("oracle still depends on a nonportable external Codex stub")
	}
}

func TestProdex04382OraclePinsEveryPublishedPlatformAndRejectsFallback(t *testing.T) {
	seen := map[string]bool{}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			asset, digest, err := prodexPlatformPin04382(goos, arch)
			if err != nil || !strings.HasPrefix(asset, "prodex-") || len(digest) != 64 || seen[digest] {
				t.Fatalf("invalid or duplicate pin %s/%s", goos, arch)
			}
			if strings.HasSuffix(asset, ".exe") != (goos == "windows") {
				t.Fatal("platform executable suffix changed")
			}
			seen[digest] = true
		}
	}
	if _, _, err := prodexPlatformPin04382("freebsd", "amd64"); err == nil {
		t.Fatal("unsupported platform silently reused a different executable")
	}
	if err := verifyProdexManifest04382(filepath.Join(t.TempDir(), "missing"), "synthetic"); err == nil {
		t.Fatal("missing official manifest accepted")
	}
	path := filepath.Join(t.TempDir(), "release-manifest.json")
	if err := os.WriteFile(path, []byte(`{"version":"0.438.2"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyProdexManifest04382(path, "synthetic"); err == nil {
		t.Fatal("modified official manifest accepted")
	}
}
