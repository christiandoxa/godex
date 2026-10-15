package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// A brand-new lock is coordination metadata, not an auth backoff snapshot.
// The latter still fails this credential-free fixture's independent state
// audit. Do not broaden the allowlist to arbitrary routing auth artifacts.
func TestProdex043714DifferentialAuthGuardAllowsOnlyPrivateEmptyLock(t *testing.T) {
	if !allowedFixtureStateFile("state/routing-auth-backoff.guard") {
		t.Fatal("known empty auth coordination lock rejected")
	}
	for _, name := range []string{"state/auth-backoff.json", "state/auth-backoff.json.last-good",
		"state/routing-auth-backoff.guard.debug", "state/routing-auth-backoff.guard/evil"} {
		if allowedFixtureStateFile(name) {
			t.Fatalf("unexpected auth state path allowlisted: %s", name)
		}
	}
	root := fixtureStateRoot(t)
	path := filepath.Join(root, "state", "routing-auth-backoff.guard")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if issues := auditFixtureDurableState(root); len(issues) != 0 {
		t.Fatalf("private coordination lock rejected: %v", issues)
	}
	if err := os.WriteFile(path, []byte("secret-or-state"), 0600); err != nil {
		t.Fatal(err)
	}
	if issues := auditFixtureDurableState(root); !slices.Contains(issues, "auth_guard_invalid") {
		t.Fatalf("nonempty auth coordination lock was allowed: %v", issues)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if issues := auditFixtureDurableState(root); !slices.Contains(issues, "auth_guard_invalid") {
			t.Fatalf("public auth coordination lock was allowed: %v", issues)
		}
	}
}
