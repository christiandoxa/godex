package codex

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestPersistSessionGoalAttachmentPathsRewritesAllGoalRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared-codex")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	oldRoot := filepath.Join(t.TempDir(), "deleted-overlay")
	source := filepath.Join(oldRoot, "attachments", "thread-1", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("goal attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	if _, err := db.Exec(
		"INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)",
		"thread-1", "read "+source,
	); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.WriteFile(filepath.Join(root, "goals_invalid.sqlite"), []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := persistSessionGoalAttachmentPaths(root); err != nil {
		t.Fatal(err)
	}

	stable := filepath.Join(root, "attachments", "thread-1", "pasted-text-1.txt")
	if got, err := os.ReadFile(stable); err != nil || string(got) != "goal attachment" {
		t.Fatalf("stable attachment = %q, err=%v", got, err)
	}
	objective := readGoalObjective(t, dbPath, "thread-1")
	if !strings.Contains(objective, stable) || strings.Contains(objective, oldRoot) {
		t.Fatalf("rewritten objective = %q", objective)
	}
}

func TestPersistSessionGoalAttachmentPathsSkipsSymlinkedDatabase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior covered on Unix")
	}
	root := filepath.Join(t.TempDir(), "shared-codex")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	oldRoot := filepath.Join(t.TempDir(), "deleted-overlay")
	oldPath := filepath.Join(oldRoot, "attachments", "thread-1", "goal-objective.md")
	stable := filepath.Join(root, "attachments", "thread-1", "goal-objective.md")
	if err := os.MkdirAll(filepath.Dir(stable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stable, []byte("stable"), 0o600); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "outside.sqlite")
	db := createGoalDB(t, outside)
	if _, err := db.Exec(
		"INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)",
		"thread-1", "read "+oldPath,
	); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.Symlink(outside, filepath.Join(root, "goals_1.sqlite")); err != nil {
		t.Fatal(err)
	}

	if err := persistSessionGoalAttachmentPaths(root); err != nil {
		t.Fatal(err)
	}
	if got := readGoalObjective(t, outside, "thread-1"); got != "read "+oldPath {
		t.Fatalf("symlinked goal DB changed: %q", got)
	}
}

func TestPersistSessionGoalAttachmentPathForThreadOnlyUpdatesRequestedThread(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared-codex")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	oldRoot := t.TempDir()
	for _, thread := range []string{"thread-1", "thread-2"} {
		source := filepath.Join(oldRoot, "attachments", thread, "pasted-text-1.txt")
		if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte(thread), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(
			"INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)",
			thread, "read "+source,
		); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	if err := persistSessionGoalAttachmentPathForThread(root, "thread-1"); err != nil {
		t.Fatal(err)
	}

	first := readGoalObjective(t, dbPath, "thread-1")
	second := readGoalObjective(t, dbPath, "thread-2")
	if !strings.Contains(first, filepath.Join(root, "attachments", "thread-1", "pasted-text-1.txt")) {
		t.Fatalf("thread-1 objective = %q", first)
	}
	if !strings.Contains(second, filepath.Join(oldRoot, "attachments", "thread-2", "pasted-text-1.txt")) {
		t.Fatalf("thread-2 objective unexpectedly changed = %q", second)
	}
}

func TestSessionThreadIDMatchesProdexPathPrecedenceAndTrim(t *testing.T) {
	content := strings.Join([]string{
		"not-json",
		"{\"payload\":{\"thread_id\":\"  payload-snake  \",\"threadId\":\"payload-camel\"},\"thread_id\":\"root-snake\"}",
		"{\"thread_id\":\"later\"}",
	}, "\n")
	if got, ok := sessionThreadID(content); !ok || got != "payload-snake" {
		t.Fatalf("thread id = %q, found=%t", got, ok)
	}
	content = "{\"payload\":{\"thread_id\":\"   \",\"threadId\":\" payload-camel \"},\"thread_id\":\"root\"}"
	if got, ok := sessionThreadID(content); !ok || got != "payload-camel" {
		t.Fatalf("thread id fallback = %q, found=%t", got, ok)
	}
	if got, ok := sessionThreadID("{\"thread_id\":7}"); ok || got != "" {
		t.Fatalf("non-string thread id = %q, found=%t", got, ok)
	}
}

func createGoalDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE thread_goals (thread_id TEXT PRIMARY KEY, objective TEXT NOT NULL)"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func readGoalObjective(t *testing.T, path, threadID string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var objective string
	if err := db.QueryRow("SELECT objective FROM thread_goals WHERE thread_id = ?1", threadID).Scan(&objective); err != nil {
		t.Fatal(err)
	}
	return objective
}
