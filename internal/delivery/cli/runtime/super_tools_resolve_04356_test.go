package runtime

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
)

func TestProdex04356SuperToolResolverUsesNewestManagedPluginDirectories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GODEX_OPTIMIZERS_HOME", root)
	t.Setenv("PRODEX_OPTIMIZERS_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())

	for _, version := range []string{"4.12.0", "4.13.0", "4.14.0-beta.1"} {
		caveman := filepath.Join(root, "caveman", version)
		if err := os.MkdirAll(caveman, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(caveman, "AGENTS.md"), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
		ponytail := filepath.Join(root, "ponytail", version, ".codex-plugin")
		if err := os.MkdirAll(ponytail, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ponytail, "plugin.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	caveman, ok := defaultSuperToolLookup("caveman")
	if !ok || filepath.Base(caveman) != "4.13.0" {
		t.Fatalf("Caveman managed resolution = %q, %t", caveman, ok)
	}
	ponytail, ok := defaultSuperToolLookup("ponytail")
	if !ok || filepath.Base(ponytail) != "4.13.0" {
		t.Fatalf("Ponytail managed resolution = %q, %t", ponytail, ok)
	}
}

func TestProdex04356SuperToolResolverUsesManagedCommandsBeforePATH(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	root := t.TempDir()
	t.Setenv("GODEX_OPTIMIZERS_HOME", root)
	pathDir := t.TempDir()
	t.Setenv("PATH", pathDir)

	managedRTK := writeSuperResolverExecutable(t, filepath.Join(root, "rtk"), "#!/bin/sh\nprintf 'rtk 0.50.0\\n'\n")
	_ = writeSuperResolverExecutable(t, filepath.Join(pathDir, "rtk"), "#!/bin/sh\nprintf 'rtk 0.50.0\\n'\n")
	got, ok := defaultSuperToolLookup("rtk")
	if !ok || got != managedRTK {
		t.Fatalf("RTK resolution = %q, %t; want %q", got, ok, managedRTK)
	}

	checkout := filepath.Join(root, "codebase-memory-mcp", "build", "c")
	managedMemory := writeSuperResolverExecutable(t, filepath.Join(checkout, "codebase-memory-mcp"), "#!/bin/sh\nprintf 'codebase-memory-mcp 0.9.1\\n'\n")
	got, ok = defaultSuperToolLookup("codebase-memory-mcp")
	if !ok || got != managedMemory {
		t.Fatalf("Codebase Memory resolution = %q, %t; want %q", got, ok, managedMemory)
	}
}

func TestProdex04356SuperToolResolverPlaywrightRequiresNode18AndNoInstallPackageProbe(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	node := filepath.Join(bin, "node")
	npx := filepath.Join(bin, "npx")
	if err := os.WriteFile(node, []byte("#!/bin/sh\nprintf 'v20.11.1\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(t.TempDir(), "npx.args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + superResolverShellQuote(capture) + "\nprintf '0.0.83\\n'\n"
	if err := os.WriteFile(npx, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	got, ok := defaultSuperToolLookup("playwright-mcp")
	if !ok || got != npx {
		t.Fatalf("Playwright resolution = %q, %t", got, ok)
	}
	args, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "--no-install @playwright/mcp --version\n" {
		t.Fatalf("Playwright probe args = %q", args)
	}

	if err := os.WriteFile(node, []byte("#!/bin/sh\nprintf 'v16.20.0\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, ok := defaultSuperToolLookup("playwright-mcp"); ok || got != "" {
		t.Fatalf("old Node unexpectedly resolved Playwright: %q, %t", got, ok)
	}
}

func writeSuperResolverExecutable(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if body == "" {
		body = "#!/bin/sh\nexit 0\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func superResolverShellQuote(value string) string {
	return "'" + value + "'"
}

func TestProdex04356SuperToolResolverEnforcesTaggedMinimumVersions(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	root := t.TempDir()
	t.Setenv("GODEX_OPTIMIZERS_HOME", root)
	t.Setenv("PRODEX_OPTIMIZERS_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())

	oldCaveman := filepath.Join(root, "caveman", "2.3.0")
	if err := os.MkdirAll(oldCaveman, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldCaveman, "AGENTS.md"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldPonytail := filepath.Join(root, "ponytail", "4.8.9", ".codex-plugin")
	if err := os.MkdirAll(oldPonytail, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldPonytail, "plugin.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if path, ok := defaultSuperToolLookup("caveman"); ok || path != "" {
		t.Fatalf("old Caveman resolved: %q %t", path, ok)
	}
	if path, ok := defaultSuperToolLookup("ponytail"); ok || path != "" {
		t.Fatalf("old Ponytail resolved: %q %t", path, ok)
	}

	rtk := writeSuperResolverExecutable(t, filepath.Join(root, "rtk"), "#!/bin/sh\nprintf 'rtk 0.45.9\\n'\n")
	if path, ok := defaultSuperToolLookup("rtk"); ok || path != "" {
		t.Fatalf("old RTK resolved from %q: %q %t", rtk, path, ok)
	}
	if err := os.WriteFile(rtk, []byte("#!/bin/sh\nprintf 'rtk 0.46.0\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if path, ok := defaultSuperToolLookup("rtk"); !ok || path != rtk {
		t.Fatalf("minimum RTK resolution = %q %t", path, ok)
	}

	codebase := writeSuperResolverExecutable(t, filepath.Join(root, "codebase-memory-mcp"), "#!/bin/sh\nprintf 'codebase-memory-mcp 0.9.0\\n'\n")
	if path, ok := defaultSuperToolLookup("codebase-memory-mcp"); ok || path != "" {
		t.Fatalf("old Codebase Memory resolved from %q: %q %t", codebase, path, ok)
	}
	if err := os.WriteFile(codebase, []byte("#!/bin/sh\nprintf 'codebase-memory-mcp 0.9.1-rc.1\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if path, ok := defaultSuperToolLookup("codebase-memory-mcp"); !ok || path != codebase {
		t.Fatalf("minimum Codebase Memory resolution = %q %t", path, ok)
	}
}

func TestProdex04356SuperSemverPrereleaseOrderingMatchesPolicy(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"0.9.1-rc.1", "0.9.1-rc.1", 0},
		{"0.9.1", "0.9.1-rc.1", 1},
		{"0.9.1-rc.2", "0.9.1-rc.1", 1},
		{"0.9.1-rc.1", "0.9.1", -1},
		{"4.13.0", "4.9.0", 1},
	}
	for _, testCase := range cases {
		got := compareSuperSemver(testCase.left, testCase.right)
		if got < 0 {
			got = -1
		} else if got > 0 {
			got = 1
		}
		if got != testCase.want {
			t.Fatalf("compare %s %s = %d, want %d", testCase.left, testCase.right, got, testCase.want)
		}
	}
}
