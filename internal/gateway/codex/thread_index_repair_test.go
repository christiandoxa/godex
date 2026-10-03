package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReconcileThreadIndexUsesSelectedHomeAndSharedSQLiteHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX shell")
	}
	root := t.TempDir()
	record := filepath.Join(root, "record")
	script := filepath.Join(root, "codex")
	activeHome := filepath.Join(root, "active-home")
	sharedHome := filepath.Join(root, "shared-home")
	if err := os.MkdirAll(activeHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sharedHome, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(sharedHome, "sessions"), filepath.Join(activeHome, "sessions")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	content := strings.Join([]string{
		"#!/bin/sh",
		"[ \"$1\" = app-server ] || exit 41",
		"printf '%s|%s|%s' \"$1\" \"$CODEX_HOME\" \"$CODEX_SQLITE_HOME\" > \"$GODEX_THREAD_INDEX_RECORD\"",
		"read line", "printf '%s\\n' '{\"id\":1,\"result\":{}}'",
		"read line", "read line", "printf '%s\\n' '{\"id\":2,\"result\":{\"nextCursor\":null}}'",
		"read line", "printf '%s\\n' '{\"id\":3,\"result\":{\"nextCursor\":null}}'",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GODEX_THREAD_INDEX_RECORD", record)
	t.Setenv("CODEX_HOME", filepath.Join(root, "ambient-home"))
	t.Setenv("CODEX_SQLITE_HOME", filepath.Join(root, "ambient-sqlite-home"))
	if err := NewCodexProcess(script, Terminal{}).ReconcileThreadIndex(t.Context(), activeHome, sharedHome); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "app-server|"+activeHome+"|"+sharedHome {
		t.Fatalf("child environment = %q", got)
	}
}

func TestThreadIndexEnvironmentMatchesProdexSamePathFallback(t *testing.T) {
	root := t.TempDir()
	ambient := filepath.Join(root, "ambient-sqlite")
	t.Setenv("CODEX_SQLITE_HOME", ambient)
	active := filepath.Join(root, "active")
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(filepath.Join(active, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shared, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	assertThreadIndexSQLiteHome(t, codexThreadIndexEnvironment(active, shared), ambient)

	missingHome := filepath.Join(root, "same")
	assertThreadIndexSQLiteHome(t, codexThreadIndexEnvironment(missingHome, missingHome), missingHome)

	sharedAliasTarget := filepath.Join(root, "alias-target")
	activeAlias := filepath.Join(root, "alias")
	if err := os.MkdirAll(sharedAliasTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sharedAliasTarget, activeAlias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	assertThreadIndexSQLiteHome(t, codexThreadIndexEnvironment(activeAlias, sharedAliasTarget), ambient)
}

func assertThreadIndexSQLiteHome(t *testing.T, environment []string, want string) {
	t.Helper()
	var got string
	for _, entry := range environment {
		if key, value, found := strings.Cut(entry, "="); found && strings.EqualFold(key, "CODEX_SQLITE_HOME") {
			got = value
		}
	}
	if got != want {
		t.Fatalf("CODEX_SQLITE_HOME = %q, want %q", got, want)
	}
}

func TestReconcileThreadIndexHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX shell")
	}
	root := t.TempDir()
	script := filepath.Join(root, "codex")
	content := "#!/bin/sh\nread line\nprintf '%s\\n' '{\"id\":1,\"result\":{}}'\nread line\nsleep 20\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := NewCodexProcess(script, Terminal{}).ReconcileThreadIndex(ctx, filepath.Join(root, "active"), "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("reconciliation error = %v", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("cleanup exceeded bound: %s", time.Since(started))
	}
}
