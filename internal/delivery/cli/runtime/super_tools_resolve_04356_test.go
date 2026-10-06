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
	root := t.TempDir()
	t.Setenv("GODEX_OPTIMIZERS_HOME", root)
	pathDir := t.TempDir()
	t.Setenv("PATH", pathDir)

	managedRTK := writeSuperResolverExecutable(t, filepath.Join(root, "rtk"), "")
	_ = writeSuperResolverExecutable(t, filepath.Join(pathDir, "rtk"), "")
	got, ok := defaultSuperToolLookup("rtk")
	if !ok || got != managedRTK {
		t.Fatalf("RTK resolution = %q, %t; want %q", got, ok, managedRTK)
	}

	checkout := filepath.Join(root, "codebase-memory-mcp", "build", "c")
	managedMemory := writeSuperResolverExecutable(t, filepath.Join(checkout, "codebase-memory-mcp"), "")
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
