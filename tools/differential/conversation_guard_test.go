package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConversationGuardRequiresEmptyPrivateLock(t *testing.T) {
	for _, fixture := range []struct {
		name, content string
		mode          os.FileMode
		valid         bool
	}{
		{"empty", "", 0o600, true},
		{"populated", "unexpected", 0o600, false},
		{"public", "", 0o644, runtime.GOOS == "windows"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := fixtureStateRoot(t)
			path := filepath.Join(root, "state", "routing-conversation.guard")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(fixture.content), fixture.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, fixture.mode); err != nil {
				t.Fatal(err)
			}
			issues := auditFixtureDurableState(root)
			if (len(issues) == 0) != fixture.valid {
				t.Fatalf("guard validity=%v, want %v: %v", len(issues) == 0, fixture.valid, issues)
			}
		})
	}
}
