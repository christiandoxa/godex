package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProdex04382FixtureSecurityDiagnosticsNeverReadFileContents(t *testing.T) {
	root := t.TempDir()
	for i := range 40 {
		path := filepath.Join(root, fmt.Sprintf("profile-%02d", i))
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "auth.json"), []byte("private-fixture-value"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	writeFixtureSecurityMetadata04382(&output, root)
	if !strings.Contains(output.String(), "auth.json") || strings.Contains(output.String(), "private-fixture-value") || strings.Count(output.String(), "fixture_security") > 44 {
		t.Fatal("security diagnostics read secret content or exceeded the metadata bound")
	}
}

func TestProdex04382OracleFailureLogsAreBoundedAndRedacted(t *testing.T) {
	root := t.TempDir()
	logs := filepath.Join(root, "prodex", "state", "logs")
	if err := os.MkdirAll(logs, 0700); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		data := "fixture-log Authorization: Bearer private-fixture-value\n" + strings.Repeat("x", 32<<10) + "out-of-bounds"
		if err := os.WriteFile(filepath.Join(logs, fmt.Sprintf("prodex-runtime-%d.log", i)), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	writeProdexFailureLogs04382(&output, root)
	if strings.Count(output.String(), "fixture-log") != 4 || output.Len() > 4*(16<<10+1) || strings.Contains(output.String(), "private-fixture-value") || strings.Contains(output.String(), "out-of-bounds") {
		t.Fatal("native failure diagnostics exceeded bounds or exposed credentials")
	}
}

func TestProdex04382FixtureRewriteRestoresPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("protected Windows ACLs are checked by the native Windows regression")
	}
	for _, kind := range []string{"auth", "state"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), kind+".json")
			if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0644); err != nil {
				t.Fatal(err)
			}
			var err error
			if kind == "auth" {
				err = writeAuth(path, "synthetic-account", "private-fixture-value")
			} else {
				err = writeJSONFile(path, map[string]any{"generation": 1})
			}
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("fixture rewrite left a secret or state file readable by other users")
			}
		})
	}
}

func TestProdex04382OracleCanonicalizesNativeTempRoot(t *testing.T) {
	parent := t.TempDir()
	link := filepath.Join(parent, "linked-temp")
	if err := os.Symlink(parent, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("creating a symlink requires a privileged Windows account")
		}
		t.Fatal(err)
	}
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, link)
	}
	root, err := createFixtureRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if root != canonical {
		t.Fatal("fixture path retains a symlink rejected by the official secret store")
	}
}

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
