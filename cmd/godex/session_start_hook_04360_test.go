package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProdex04360HiddenSessionStartHookRunsBeforeCredentialConfig(t *testing.T) {
	dir, err := os.MkdirTemp("", "godex-session-hook-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "session.id")
	old := os.Args
	os.Args = []string{"godex", "__runtime-goal-session-notify", path,
		`{"thread-id":"019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"}`}
	t.Cleanup(func() { os.Args = old })
	t.Setenv("GODEX_HOME", filepath.Join(t.TempDir(), "this-dir-has-no-config"))
	if code := run(); code != 0 {
		t.Fatalf("native SessionStart callback before config returned %d", code)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9" {
		t.Fatalf("session hook persisted incorrect UUID: %q", content)
	}
}
