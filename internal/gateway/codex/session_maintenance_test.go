package codex

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMaintainManagedSessionsMatchesProdexFullSequence(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared-codex")
	cacheRoot := filepath.Join(root, "prodex")
	sessionID := "01900000-0000-7000-8000-000000000031"
	session := filepath.Join(shared, "sessions", "2026", "10", "03", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
		t.Fatal(err)
	}

	oldRoot := filepath.Join(root, "deleted-overlay")
	oldAttachment := filepath.Join(oldRoot, "attachments", "thread-1", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(oldAttachment), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldAttachment, []byte("attachment body"), 0o600); err != nil {
		t.Fatal(err)
	}
	event, err := json.Marshal(map[string]any{
		"timestamp": "2026-10-03T12:49:50Z",
		"type":      "event",
		"payload":   map[string]any{"message": "read " + oldAttachment},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := string(event) + "\n" +
		"{\"timestamp\":\"2026-10-03T12:50:00Z\",\"type\":\"session_meta\",\"payload\":{\"id\":\"" + sessionID + "\",\"thread_id\":\"thread-1\"}}\n"
	if err := os.WriteFile(session, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	archived := filepath.Join(shared, "archived_sessions", "2026", "10", "02", "rollout-archived.jsonl")
	if err := os.MkdirAll(filepath.Dir(archived), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archived, []byte("{\"timestamp\":\"2026-10-02T09:00:00Z\",\"type\":\"session_meta\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(shared, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	if _, err := db.Exec(
		"INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)",
		"thread-1", "read "+oldAttachment,
	); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := MaintainManagedSessions(shared, cacheRoot); err != nil {
		t.Fatal(err)
	}

	rewritten, err := os.ReadFile(session)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(rewritten)), "\n")
	if len(lines) != 2 || !sessionLineStartsCodexRolloutMetadata(lines[0]) {
		t.Fatalf("session metadata prefix = %#v", lines)
	}
	stable := filepath.Join(shared, "attachments", "thread-1", "pasted-text-1.txt")
	var rewrittenEvent map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &rewrittenEvent); err != nil {
		t.Fatal(err)
	}
	payload, _ := rewrittenEvent["payload"].(map[string]any)
	message, _ := payload["message"].(string)
	if message != "read "+stable || strings.Contains(message, oldRoot) {
		t.Fatalf("session attachment path not stabilized: %s", rewritten)
	}
	if got, err := os.ReadFile(stable); err != nil || string(got) != "attachment body" {
		t.Fatalf("stable attachment = %q, err=%v", got, err)
	}
	if _, err := os.Stat(sessionRepairBackupPath(session)); err != nil {
		t.Fatalf("metadata repair backup missing: %v", err)
	}
	if got := readGoalObjective(t, dbPath, "thread-1"); !strings.Contains(got, stable) || strings.Contains(got, oldRoot) {
		t.Fatalf("goal objective = %q", got)
	}

	cache := loadSessionMaintenanceCache(filepath.Join(cacheRoot, sessionMaintenanceCacheFile))
	if cache.Version != sessionMaintenanceCacheVersion {
		t.Fatalf("cache version = %d", cache.Version)
	}
	for _, path := range []string{session, archived} {
		key, err := filepath.Rel(shared, path)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := cache.Files[key]; !ok {
			t.Fatalf("cache missing %q: %#v", key, cache.Files)
		}
	}
	info, err := os.Stat(session)
	if err != nil {
		t.Fatal(err)
	}
	wantMTime := time.Date(2026, 10, 3, 12, 49, 50, 0, time.UTC)
	if !info.ModTime().Equal(wantMTime) {
		t.Fatalf("session mtime = %s, want %s", info.ModTime(), wantMTime)
	}
}

func TestMaintainManagedSessionsDoesNotCacheUnstableAttachmentPath(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	cacheRoot := filepath.Join(root, "prodex")
	session := filepath.Join(shared, "sessions", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing", "attachments", "thread", "pasted-text-1.txt")
	raw := "{\"timestamp\":\"2026-10-03T10:00:00Z\",\"type\":\"session_meta\",\"payload\":{\"message\":\"read " + missing + "\"}}\n"
	if err := os.WriteFile(session, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MaintainManagedSessions(shared, cacheRoot); err != nil {
		t.Fatal(err)
	}
	cache := loadSessionMaintenanceCache(filepath.Join(cacheRoot, sessionMaintenanceCacheFile))
	key, _ := filepath.Rel(shared, session)
	if _, ok := cache.Files[key]; ok {
		t.Fatalf("unstable session entered maintenance cache: %#v", cache.Files)
	}
}

func TestMaintainManagedSessionsSkipsWhenChildHoldsSessionLock(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	cacheRoot := filepath.Join(root, "prodex")
	session := filepath.Join(shared, "sessions", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := "{\"timestamp\":\"2026-10-03T10:00:00Z\",\"type\":\"session_meta\"}\n"
	if err := os.WriteFile(session, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := (SessionLocker{}).LockCodexSessionsForChild(context.Background(), shared)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	if err := MaintainManagedSessions(shared, cacheRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, sessionMaintenanceCacheFile)); !os.IsNotExist(err) {
		t.Fatalf("contended maintenance wrote cache: %v", err)
	}
}

func TestMaintainManagedSessionsStillRewritesGoalsOnSessionCacheHit(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	cacheRoot := filepath.Join(root, "prodex")
	session := filepath.Join(shared, "sessions", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte("{\"timestamp\":\"2026-10-03T10:00:00Z\",\"type\":\"session_meta\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MaintainManagedSessions(shared, cacheRoot); err != nil {
		t.Fatal(err)
	}

	oldRoot := t.TempDir()
	oldPath := filepath.Join(oldRoot, "attachments", "thread-1", "goal-objective.md")
	stable := filepath.Join(shared, "attachments", "thread-1", "goal-objective.md")
	if err := os.MkdirAll(filepath.Dir(stable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stable, []byte("stable"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(shared, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	if _, err := db.Exec("INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)", "thread-1", "read "+oldPath); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := MaintainManagedSessions(shared, cacheRoot); err != nil {
		t.Fatal(err)
	}
	if got := readGoalObjective(t, dbPath, "thread-1"); !strings.Contains(got, stable) {
		t.Fatalf("cached-session maintenance skipped goal rewrite: %q", got)
	}
}

func openGoalDBForMaintenanceTest(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
