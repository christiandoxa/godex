package codex

import (
	"context"
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

func TestPersistSessionGoalObjectiveAttachmentsRepairsGeneratedWrapper(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared-codex")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	oldRoot := filepath.Join(t.TempDir(), "deleted-overlay")
	nested := filepath.Join(oldRoot, "attachments", "thread-1", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(nested), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("nested bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(root, "attachments", "thread-1", "goal-objective.md")
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte("read "+nested), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(root, "attachments", "thread-1", "pasted-text-2.txt")
	if err := os.WriteFile(raw, []byte("raw "+nested), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := persistSessionGoalObjectiveAttachments(root, "read "+wrapper); err != nil {
		t.Fatal(err)
	}
	stableNested := filepath.Join(root, "attachments", "thread-1", "pasted-text-1.txt")
	got, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "read "+stableNested {
		t.Fatalf("generated wrapper = %q", got)
	}
	if nestedBytes, err := os.ReadFile(stableNested); err != nil || string(nestedBytes) != "nested bytes" {
		t.Fatalf("stable nested attachment = %q, err=%v", nestedBytes, err)
	}
	if rawBytes, err := os.ReadFile(raw); err != nil || string(rawBytes) != "raw "+nested {
		t.Fatalf("raw pasted attachment changed = %q, err=%v", rawBytes, err)
	}
}

func TestPersistSessionGoalObjectiveAttachmentsRejectsSymlinkedWrapper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior is covered on Unix")
	}
	root := filepath.Join(t.TempDir(), "shared-codex")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-objective.md")
	if err := os.WriteFile(outside, []byte("raw objective"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(root, "attachments", "thread-1", "goal-objective.md")
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, wrapper); err != nil {
		t.Fatal(err)
	}
	if err := persistSessionGoalObjectiveAttachments(root, "read "+wrapper); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != "raw objective" {
		t.Fatalf("symlink target changed = %q, err=%v", got, err)
	}
}

func TestUpdateSessionGoalAttachmentReferencePreservesNewerObjective(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	defer db.Close()
	if _, err := db.Exec(
		"INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)",
		"thread-1", "new objective",
	); err != nil {
		t.Fatal(err)
	}
	if err := updateSessionGoalAttachmentReference(
		db, dbPath, "thread-1", "old objective", "rewritten objective",
	); err != nil {
		t.Fatal(err)
	}
	if got := readGoalObjective(t, dbPath, "thread-1"); got != "new objective" {
		t.Fatalf("newer objective was overwritten: %q", got)
	}
}

func TestMaintainManagedSessionsRepairsGoalsBeforeContendedRolloutLock(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	oldRoot := filepath.Join(root, "deleted-overlay")
	oldPath := filepath.Join(oldRoot, "attachments", "thread-1", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("goal bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(shared, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	if _, err := db.Exec(
		"INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)",
		"thread-1", "read "+oldPath,
	); err != nil {
		t.Fatal(err)
	}
	db.Close()

	release, err := (SessionLocker{}).LockCodexSessionsForChild(context.Background(), shared)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := MaintainManagedSessions(shared, filepath.Join(root, "cache")); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(shared, "attachments", "thread-1", "pasted-text-1.txt")
	if got := readGoalObjective(t, dbPath, "thread-1"); got != "read "+stable {
		t.Fatalf("contended maintenance did not repair goal: %q", got)
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
	content = "{\"type\":\"session_meta\",\"payload\":{\"id\":\" native-session \"}}"
	if got, ok := sessionThreadID(content); !ok || got != "native-session" {
		t.Fatalf("native session metadata id = %q, found=%t", got, ok)
	}
}

func TestMaintainManagedSessionsRepairsNativeSessionGoalAttachment(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	cacheRoot := filepath.Join(root, "cache")
	sessionID := "01900000-0000-7000-8000-000000000032"
	session := filepath.Join(shared, "sessions", "2026", "10", "03", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
		t.Fatal(err)
	}
	oldRoot := filepath.Join(root, "deleted-overlay")
	oldAttachment := filepath.Join(oldRoot, "attachments", sessionID, "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(oldAttachment), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldAttachment, []byte("native goal attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := `{"type":"event","payload":{"message":"read ` + oldAttachment + `"}}` + "\n" +
		`{"type":"session_meta","payload":{"id":"` + sessionID + `"}}` + "\n"
	if err := os.WriteFile(session, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(shared, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	if _, err := db.Exec(
		"INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)",
		sessionID, "read "+oldAttachment,
	); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := MaintainManagedSessions(shared, cacheRoot); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(shared, "attachments", sessionID, "pasted-text-1.txt")
	if got, err := os.ReadFile(stable); err != nil || string(got) != "native goal attachment" {
		t.Fatalf("stable native attachment = %q, err=%v", got, err)
	}
	objective := readGoalObjective(t, dbPath, sessionID)
	if !strings.Contains(objective, stable) || strings.Contains(objective, oldRoot) {
		t.Fatalf("native goal objective = %q", objective)
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

func TestPrepareSharedSessionHomeRepairsGoalsWhenProfileIsSharedRoot(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(t.TempDir(), "deleted-overlay")
	stale := filepath.Join(oldRoot, "attachments", "thread-picker", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("picker goal"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	if _, err := db.Exec("INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)", "thread-picker", "read "+stale); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := NewCodexProcess("codex", Terminal{}).PrepareSharedSessionHome(root, root); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(root, "attachments", "thread-picker", "pasted-text-1.txt")
	if got := readGoalObjective(t, dbPath, "thread-picker"); got != "read "+stable {
		t.Fatalf("picker objective = %q, want stable attachment", got)
	}
	if got, err := os.ReadFile(stable); err != nil || string(got) != "picker goal" {
		t.Fatalf("stable picker attachment = %q, err=%v", got, err)
	}
}
