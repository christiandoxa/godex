package claude

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClaudeSourceLoadsNestedAndTopLevelCredentials(t *testing.T) {
	for _, fixture := range []struct {
		name, content, account, method string
	}{
		{"nested", `{"claudeAiOauth":{"accessToken":"fixture-access","subscriptionType":"pro","email":"person@example.test"}}`, "person@example.test", "claude-ai-oauth:pro"},
		{"top-level", `{"accessToken":"fixture-access","subscriptionType":"max","email":"person@example.test"}`, "person@example.test", "claude-ai-oauth:max"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, CredentialsFile), []byte(fixture.content), 0o644); err != nil {
				t.Fatal(err)
			}
			source := &Source{homeDir: func() (string, error) { return t.TempDir(), nil }, getenv: func(string) string { return dir }}
			credential, err := source.Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if credential.Provider.Kind != "anthropic" || credential.Provider.Account == nil || *credential.Provider.Account != fixture.account || credential.Provider.AuthMethod == nil || *credential.Provider.AuthMethod != fixture.method {
				t.Fatalf("credential = %#v", credential)
			}
			if len(credential.SecretFiles) != 1 || credential.SecretFiles[0].Path != CredentialsFile || credential.SecretFiles[0].Text != fixture.content {
				t.Fatalf("secret files = %#v", credential.SecretFiles)
			}
		})
	}
}

func TestClaudeSourceUsesHomeDefaultAndRejectsUnsafeSources(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, CredentialsFile), []byte(`{"accessToken":"fixture-access"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	source := &Source{homeDir: func() (string, error) { return home, nil }, getenv: func(string) string { return "" }}
	if _, err := source.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	bad := &Source{homeDir: func() (string, error) { return home, nil }, getenv: func(string) string { return "../claude" }}
	if _, err := bad.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unsafe path error = %v", err)
	}

	if runtime.GOOS != "windows" {
		real := t.TempDir()
		if err := os.WriteFile(filepath.Join(real, CredentialsFile), []byte(`{"accessToken":"fixture-access"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		linked := filepath.Join(t.TempDir(), "linked")
		if err := os.Symlink(real, linked); err != nil {
			t.Fatal(err)
		}
		symlinkSource := &Source{homeDir: func() (string, error) { return home, nil }, getenv: func(string) string { return linked }}
		if _, err := symlinkSource.Load(context.Background()); err == nil {
			t.Fatal("symlinked Claude config root unexpectedly accepted")
		}
	}
}

func TestClaudeSourceRejectsMalformedMissingAndOversizedCredentials(t *testing.T) {
	for _, content := range []string{"{}", `{"claudeAiOauth":{}}`, "{"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, CredentialsFile), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		source := &Source{homeDir: os.UserHomeDir, getenv: func(string) string { return dir }}
		if _, err := source.Load(context.Background()); err == nil {
			t.Fatalf("credentials %q unexpectedly accepted", content)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, CredentialsFile), make([]byte, credentialsMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	source := &Source{homeDir: os.UserHomeDir, getenv: func(string) string { return dir }}
	if _, err := source.Load(context.Background()); err == nil {
		t.Fatal("oversized Claude credentials unexpectedly accepted")
	}
}
