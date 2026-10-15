package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProdex043714StartupManagedAuthLocatorRejectsAmbiguousOrUnsafeFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := startupManagedAuthFile043714(root); err == nil {
		t.Fatal("missing managed login accepted")
	}
	first := filepath.Join(root, "profiles", "main", "auth.json")
	if err := os.MkdirAll(filepath.Dir(first), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte(`{"auth_mode":"chatgpt"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if selected, err := startupManagedAuthFile043714(root); err != nil || selected != first {
		t.Fatalf("valid imported managed auth rejected: %s %v", selected, err)
	}
	second := filepath.Join(root, "accounts", "synthetic", "codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(second), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`{"auth_mode":"chatgpt"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := startupManagedAuthFile043714(root); err == nil {
		t.Fatal("ambiguous profile layout accepted")
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	if selected, err := startupManagedAuthFile043714(root); err != nil || selected != second {
		t.Fatalf("valid Godex imported auth rejected: %s %v", selected, err)
	}
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.json"), second); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := startupManagedAuthFile043714(root); err == nil {
		t.Fatal("symlink auth fixture accepted")
	}
}

func TestProdex043714StartupManagedAuthLocatorRejectsAncestorSymlink(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	target := filepath.Join(external, "codex")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "auth.json"), []byte(`{"auth_mode":"chatgpt"}`), 0600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "accounts", "synthetic")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(nested, "codex")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := startupManagedAuthFile043714(root); err == nil {
		t.Fatal("symlinked managed-home ancestor accepted")
	}
}
