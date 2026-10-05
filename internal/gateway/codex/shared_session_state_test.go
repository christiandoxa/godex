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
		for _, name := range []string{"auth.json", "config.toml"} {
			info, err := os.Lstat(filepath.Join(home, name))
			if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				t.Fatalf("%s/%s lost profile locality: mode=%v err=%v", home, name, func() os.FileMode {
					if info != nil {
						return info.Mode()
					}
					return 0
				}(), err)
			}
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
