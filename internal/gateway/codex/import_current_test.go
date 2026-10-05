package codex

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestImportCurrentCopiesPrivateNativeHome(t *testing.T) {
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
	if runtime.GOOS != "windows" {
		if err := os.Chmod(sourceHome, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sourceHome, "history.jsonl"), []byte("synthetic history"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "model = \"source-model\"\n"
	if err := os.WriteFile(filepath.Join(sourceHome, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceHome, "packages", "standalone"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sourceHome, "packages", "standalone", "codex"),
		[]byte("installer-owned"), 0o700,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceHome, "skills", "packages"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sourceHome, "skills", "packages", "manifest.json"),
		[]byte("{}"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	stagedHome := filepath.Join(t.TempDir(), "staged")

	identity, err := NewCodexProcess("codex", Terminal{}).ImportCurrent(context.Background(), sourceHome, stagedHome, false)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Email != "person@example.com" || identity.ChatGPTAccountID != "account-123" {
		t.Fatalf("identity = %+v", identity)
	}
	stagedIdentity, err := readChatGPTIdentity(filepath.Join(stagedHome, "auth.json"))
	if err != nil || stagedIdentity.Email != identity.Email || stagedIdentity.ChatGPTAccountID != identity.ChatGPTAccountID {
		t.Fatalf("staged identity differs from metadata: %v", err)
	}
	if _, err := ReadAccessToken(filepath.Join(stagedHome, "auth.json")); err != nil {
		t.Fatalf("staged auth: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagedHome, "config.toml")); err != nil {
		t.Fatalf("staged config: %v", err)
	}
	for name, expected := range map[string]string{"history.jsonl": "synthetic history", "config.toml": config} {
		content, err := os.ReadFile(filepath.Join(stagedHome, name))
		if err != nil || string(content) != expected {
			t.Fatalf("copied %s = %q, err = %v", name, content, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(stagedHome, "packages")); !os.IsNotExist(err) {
		t.Fatalf("root packages unexpectedly copied: %v", err)
	}
	nestedPackages, err := os.ReadFile(
		filepath.Join(stagedHome, "skills", "packages", "manifest.json"),
	)
	if err != nil || string(nestedPackages) != "{}" {
		t.Fatalf("nested packages = %q err=%v", nestedPackages, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(stagedHome)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("staged home mode = %v", info.Mode().Perm())
		}
	}
}

func TestImportCurrentAcceptsTrustedReadableSourceDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory modes do not apply on Windows")
	}
	authPath := writeAuthForTest(t, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": jwtForTest(t, map[string]any{"chatgpt_account_id": "account-123"}),
		},
	})
	sourceHome := filepath.Dir(authPath)
	if err := os.Chmod(sourceHome, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := NewCodexProcess("codex", Terminal{}).ImportCurrent(
		context.Background(), sourceHome, filepath.Join(t.TempDir(), "staged"), false,
	); err != nil {
		t.Fatalf("trusted non-writable 0755 source home was rejected: %v", err)
	}
	info, err := os.Stat(sourceHome)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("source home mode mutated to %o", info.Mode().Perm())
	}
}

func TestImportCurrentRejectsUntrustedWritableSourceUnlessInsecure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory modes do not apply on Windows")
	}
	authPath := writeAuthForTest(t, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": "opaque",
			"account_id":   "writable-source",
		},
	})
	sourceHome := filepath.Dir(authPath)
	if err := os.Chmod(sourceHome, 0o775); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess("codex", Terminal{})
	if _, err := process.ImportCurrent(
		context.Background(), sourceHome, filepath.Join(t.TempDir(), "rejected"), false,
	); err == nil {
		t.Fatal("group-writable source home unexpectedly accepted")
	}
	info, err := os.Stat(sourceHome)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o775 {
		t.Fatalf("rejected import mutated source home mode to %o", info.Mode().Perm())
	}
	if _, err := process.ImportCurrent(
		context.Background(), sourceHome, filepath.Join(t.TempDir(), "accepted"), true,
	); err != nil {
		t.Fatalf("--insecure import rejected writable source home: %v", err)
	}
	info, err = os.Stat(sourceHome)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o775 {
		t.Fatalf("--insecure import mutated source home mode to %o", info.Mode().Perm())
	}
}

func TestImportCurrentPrivateAuthValidationNeverMutatesSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes do not apply on Windows")
	}
	authPath := writeAuthForTest(t, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": jwtForTest(t, map[string]any{"chatgpt_account_id": "account-123"}),
		},
	})
	sourceHome := filepath.Dir(authPath)
	if err := os.Chmod(sourceHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(authPath, 0o640); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess("codex", Terminal{})
	if _, err := process.ImportCurrent(
		context.Background(), sourceHome, filepath.Join(t.TempDir(), "rejected"), false,
	); err == nil {
		t.Fatal("group-readable auth.json unexpectedly accepted without --insecure")
	}
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("rejected import mutated source auth mode to %o", info.Mode().Perm())
	}

	if _, err := process.ImportCurrent(
		context.Background(), sourceHome, filepath.Join(t.TempDir(), "accepted"), true,
	); err != nil {
		t.Fatalf("--insecure import rejected trusted source: %v", err)
	}
	info, err = os.Stat(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("--insecure import mutated source auth mode to %o", info.Mode().Perm())
	}
}

func TestIdentityUsesSnapshotAfterNativeCredentialReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	old := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-old","account_id":"old-account"}}`)
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readPrivateAuthFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(snapshot)
	if err := os.WriteFile(path, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-new","account_id":"new-account"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := chatGPTIdentity(snapshot)
	if err != nil || identity.ChatGPTAccountID != "old-account" {
		t.Fatalf("identity did not follow credential snapshot: %v", err)
	}
}

func TestImportCurrentRejectsSymlinkHome(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "codex-home")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	process := NewCodexProcess("codex", Terminal{})
	for _, insecure := range []bool{false, true} {
		if _, err := process.ImportCurrent(context.Background(), link, filepath.Join(t.TempDir(), "staged"), insecure); err == nil {
			t.Fatalf("symlink Codex home unexpectedly accepted with insecure=%t", insecure)
		}
	}
}
