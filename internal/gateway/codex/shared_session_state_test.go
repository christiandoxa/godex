package codex

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrepareSharedSessionHomeMergesProfilesAndKeepsCredentialsLocal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation depends on host privilege; cross-build covers the implementation")
	}
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	first := filepath.Join(root, "profiles", "first")
	second := filepath.Join(root, "profiles", "second")
	for _, home := range []string{first, second} {
		if err := os.MkdirAll(filepath.Join(home, "sessions", "2026", "10", "05"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(home, "archived_sessions"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte("auth-"+filepath.Base(home)), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("config-"+filepath.Base(home)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	firstSession := filepath.Join(first, "sessions", "2026", "10", "05", "rollout-first.jsonl")
	secondSession := filepath.Join(second, "sessions", "2026", "10", "05", "rollout-second.jsonl")
	if err := os.WriteFile(firstSession, []byte("first-session"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondSession, []byte("second-session"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "state_5.sqlite"), []byte("first-index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "state_5.sqlite"), []byte("second-index"), 0o600); err != nil {
		t.Fatal(err)
	}

	process := NewCodexProcess("codex", Terminal{})
	if err := process.PrepareSharedSessionHome(first, shared); err != nil {
		t.Fatal(err)
	}
	if err := process.PrepareSharedSessionHome(second, shared); err != nil {
		t.Fatal(err)
	}

	for _, home := range []string{first, second} {
		for _, name := range []string{"sessions", "archived_sessions", "attachments", "image_attachments", "shell_snapshots"} {
			info, err := os.Lstat(filepath.Join(home, name))
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("%s/%s is not shared symlink: mode=%v err=%v", home, name, func() os.FileMode {
					if info != nil {
						return info.Mode()
					}
					return 0
				}(), err)
			}
		}
		stateInfo, err := os.Lstat(filepath.Join(home, "state_5.sqlite"))
		if err != nil || stateInfo.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s state db is not shared symlink: mode=%v err=%v", home, func() os.FileMode {
				if stateInfo != nil {
					return stateInfo.Mode()
				}
				return 0
			}(), err)
		}
		authInfo, err := os.Lstat(filepath.Join(home, "auth.json"))
		if err != nil || authInfo.Mode()&os.ModeSymlink != 0 || !authInfo.Mode().IsRegular() {
			t.Fatalf("%s/auth.json lost profile locality: mode=%v err=%v", home, func() os.FileMode {
				if authInfo != nil {
					return authInfo.Mode()
				}
				return 0
			}(), err)
		}
		configInfo, err := os.Lstat(filepath.Join(home, "config.toml"))
		if err != nil || configInfo.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s/config.toml is not shared: mode=%v err=%v", home, func() os.FileMode {
				if configInfo != nil {
					return configInfo.Mode()
				}
				return 0
			}(), err)
		}
	}
	for name, want := range map[string]string{
		"sessions/2026/10/05/rollout-first.jsonl":  "first-session",
		"sessions/2026/10/05/rollout-second.jsonl": "second-session",
		"state_5.sqlite": "first-index",
	} {
		content, err := os.ReadFile(filepath.Join(shared, filepath.FromSlash(name)))
		if err != nil || string(content) != want {
			t.Fatalf("shared %s = %q err=%v, want %q", name, content, err, want)
		}
	}
	if content, _ := os.ReadFile(filepath.Join(first, "auth.json")); string(content) != "auth-first" {
		t.Fatalf("first auth changed: %q", content)
	}
	if content, _ := os.ReadFile(filepath.Join(second, "auth.json")); string(content) != "auth-second" {
		t.Fatalf("second auth changed: %q", content)
	}
}

func TestPrepareSharedSessionHomeRejectsSharedSQLiteSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires Windows privileges")
	}
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "state_5.sqlite"), []byte("profile"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.sqlite")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(shared, "state_5.sqlite")); err != nil {
		t.Fatal(err)
	}
	if err := NewCodexProcess("codex", Terminal{}).PrepareSharedSessionHome(profile, shared); err == nil || !strings.Contains(err.Error(), "real regular file") {
		t.Fatalf("shared SQLite symlink error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(profile, "state_5.sqlite"))
	if err != nil || string(content) != "profile" {
		t.Fatalf("profile state changed after rejection: %q err=%v", content, err)
	}
}

func TestProxyChildEnvironmentUsesSharedSQLiteHomeOnlyForSharedSessions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires Windows privileges")
	}
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess("codex", Terminal{})
	if err := process.PrepareSharedSessionHome(profile, shared); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(proxyChildEnvironment(profile, "", shared), "\n")
	if !strings.Contains(joined, "CODEX_HOME="+profile) || !strings.Contains(joined, "CODEX_SQLITE_HOME="+shared) {
		t.Fatalf("shared child environment = %s", joined)
	}
	other := filepath.Join(root, "other")
	if err := os.MkdirAll(filepath.Join(other, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(proxyChildEnvironment(other, "", shared), "\n")
	if strings.Contains(joined, "CODEX_SQLITE_HOME="+shared) {
		t.Fatalf("unshared child unexpectedly got shared sqlite home: %s", joined)
	}
}

func TestPrepareSharedSessionHomePreservesLegacySymlinkDirectoryTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires Windows privileges")
	}
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	shared := filepath.Join(root, "shared")
	legacy := filepath.Join(root, "legacy-sessions")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(legacy, "2026", "10", "05"), 0o700); err != nil {
		t.Fatal(err)
	}
	legacyRollout := filepath.Join(legacy, "2026", "10", "05", "rollout-legacy.jsonl")
	if err := os.WriteFile(legacyRollout, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(legacy, filepath.Join(profile, "sessions")); err != nil {
		t.Fatal(err)
	}

	if err := NewCodexProcess("codex", Terminal{}).PrepareSharedSessionHome(profile, shared); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(legacyRollout); err != nil || string(content) != "legacy" {
		t.Fatalf("legacy symlink target was modified or removed: content=%q err=%v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(shared, "sessions", "2026", "10", "05", "rollout-legacy.jsonl")); err != nil || string(content) != "legacy" {
		t.Fatalf("shared rollout = %q err=%v", content, err)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(profile, "sessions"))
	if err != nil || !sameCodexHome(target, filepath.Join(shared, "sessions")) {
		t.Fatalf("profile sessions target = %q err=%v", target, err)
	}
}

func TestPrepareSharedSessionHomeMatchesProdexRuntimeManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires Windows privileges; cross-build covers link implementation")
	}
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, home := range []string{first, second} {
		if err := os.MkdirAll(home, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".credentials.json"), []byte("credentials-"+filepath.Base(home)), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("config-"+filepath.Base(home)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(first, "history.jsonl"), []byte("{\"ts\":2,\"text\":\"same\"}\n{\"ts\":1,\"text\":\"first\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "history.jsonl"), []byte("{\"ts\":2,\"text\":\"same\"}\n{\"ts\":3,\"text\":\"second\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "managed_config.toml"), []byte("first-managed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "managed_config.toml"), []byte("second-managed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "work.config.toml"), []byte("profile-v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(first, "memories", "one.md"):  "memory-one",
		filepath.Join(second, "rules", "two.rules"): "rule-two",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	process := NewCodexProcess("codex", Terminal{})
	if err := process.PrepareSharedSessionHome(first, shared); err != nil {
		t.Fatal(err)
	}
	if err := process.PrepareSharedSessionHome(second, shared); err != nil {
		t.Fatal(err)
	}

	wantHistory := "{\"ts\":1,\"text\":\"first\"}\n{\"ts\":2,\"text\":\"same\"}\n{\"ts\":3,\"text\":\"second\"}"
	if content, err := os.ReadFile(filepath.Join(shared, "history.jsonl")); err != nil || string(content) != wantHistory {
		t.Fatalf("shared history = %q err=%v, want %q", content, err, wantHistory)
	}
	if content, err := os.ReadFile(filepath.Join(shared, "managed_config.toml")); err != nil || string(content) != "first-managed" {
		t.Fatalf("shared managed config = %q err=%v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(shared, "config.toml")); err != nil || string(content) != "config-first" {
		t.Fatalf("shared main config = %q err=%v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(shared, "work.config.toml")); err != nil || string(content) != "profile-v2" {
		t.Fatalf("shared profile-v2 config = %q err=%v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(shared, "memories", "one.md")); err != nil || string(content) != "memory-one" {
		t.Fatalf("shared memory = %q err=%v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(shared, "rules", "two.rules")); err != nil || string(content) != "rule-two" {
		t.Fatalf("shared rule = %q err=%v", content, err)
	}

	for _, home := range []string{first, second} {
		for _, relative := range []string{"history.jsonl", "config.toml", "managed_config.toml", "work.config.toml", "memories", "rules", "AGENTS.md"} {
			info, err := os.Lstat(filepath.Join(home, relative))
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("%s/%s is not a shared symlink: mode=%v err=%v", home, relative, func() os.FileMode {
					if info != nil {
						return info.Mode()
					}
					return 0
				}(), err)
			}
		}
		credentials, err := os.Lstat(filepath.Join(home, ".credentials.json"))
		if err != nil || credentials.Mode()&os.ModeSymlink != 0 || !credentials.Mode().IsRegular() {
			t.Fatalf("%s credentials lost locality: mode=%v err=%v", home, func() os.FileMode {
				if credentials != nil {
					return credentials.Mode()
				}
				return 0
			}(), err)
		}
	}
}
