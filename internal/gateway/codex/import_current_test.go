package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestImportCurrentCopiesOnlyChatGPTAuthentication(t *testing.T) {
	authPath := writeAuthForTest(t, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": jwtForTest(t, map[string]any{
				"email":              "person@example.com",
				"chatgpt_account_id": "account-123",
			}),
		},
	})
	sourceHome := filepath.Dir(authPath)
	if err := os.WriteFile(filepath.Join(sourceHome, "history.jsonl"), []byte("synthetic history"), 0o600); err != nil {
		t.Fatal(err)
	}
	stagedHome := filepath.Join(t.TempDir(), "staged")

	identity, err := NewCodexProcess("codex", Terminal{}).ImportCurrent(context.Background(), sourceHome, stagedHome)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Email != "person@example.com" || identity.ChatGPTAccountID != "account-123" {
		t.Fatalf("identity = %+v", identity)
	}
	if _, err := ReadAccessToken(filepath.Join(stagedHome, "auth.json")); err != nil {
		t.Fatalf("staged auth: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagedHome, "config.toml")); err != nil {
		t.Fatalf("staged config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagedHome, "history.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("history was copied or returned unexpected error: %v", err)
	}
}

func TestImportCurrentRejectsSymlinkHome(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "codex-home")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := NewCodexProcess("codex", Terminal{}).ImportCurrent(context.Background(), link, filepath.Join(t.TempDir(), "staged")); err == nil {
		t.Fatal("symlink Codex home unexpectedly accepted")
	}
}
