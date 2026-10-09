package codex

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const stateDBMaintenanceHelperMode = "GODEX_TEST_STATE_DB_MAINTENANCE"

func TestStateDBRolloutRecoveryRepairsOnlyVerifiedOverlayRows(t *testing.T) {
	root := t.TempDir()
	sessionID := "019ec6c3-28a4-79f0-91f9-74a2f34b0928"
	session := filepath.Join(root, "sessions", "2026", "08", "18", "promoted-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte(fmt.Sprintf("{\"type\":\"session_meta\",\"payload\":{\"id\":\"%s\"}}\n", sessionID)), 0o600); err != nil {
		t.Fatal(err)
	}
	contentID := "019ec6c3-28a4-79f0-91f9-74a2f34b0929"
	contentSession := filepath.Join(root, "sessions", "state-db-rollout.jsonl")
	if err := os.WriteFile(contentSession, []byte("{\"type\":\"event\",\"payload\":{\"message\":\"partial\"}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stateDB := filepath.Join(root, "state_5.sqlite")
	database := openStateDBTest(t, stateDB)
	if _, err := database.Exec("CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, ".prodex-overlay-old", "sessions", "2026", "08", "18", "old-name.jsonl")
	if _, err := database.Exec("INSERT INTO threads (id, rollout_path) VALUES (?1, ?2)", sessionID, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO threads (id, rollout_path) VALUES (?1, ?2)", contentID, contentSession); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO threads (id, rollout_path) VALUES (?1, ?2)", "unrelated", filepath.Join(t.TempDir(), "outside.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if err := MaintainManagedSessions(root, filepath.Join(root, "cache")); err != nil {
		t.Fatal(err)
	}
	database = openStateDBTest(t, stateDB)
	var got string
	if err := database.QueryRow("SELECT rollout_path FROM threads WHERE id = ?1", sessionID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != session {
		t.Fatalf("repaired rollout path = %q, want %q", got, session)
	}
	if err := database.QueryRow("SELECT rollout_path FROM threads WHERE id = ?1", contentID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != contentSession {
		t.Fatalf("content-indexed rollout path = %q, want %q", got, contentSession)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	candidates, err := stateDBSessionCandidates(root)
	if err != nil || len(candidates) != 2 || candidates[0].path != session || candidates[1].path != contentSession {
		t.Fatalf("state DB candidates = %#v, err=%v", candidates, err)
	}
	content, err := os.ReadFile(contentSession)
	if err != nil || !strings.Contains(string(content), `"type":"session_meta"`) {
		t.Fatalf("state DB session metadata = %q, err=%v", content, err)
	}
}

func TestStateDBMaintenanceSkipsAcrossProcessesWhileChildOwnsSessionLock(t *testing.T) {
	if os.Getenv(stateDBMaintenanceHelperMode) != "" {
		home := os.Getenv("GODEX_TEST_STATE_DB_HOME")
		if err := MaintainManagedSessions(home, filepath.Join(home, "cache")); err != nil {
			_, _ = fmt.Fprintln(os.Stdout, "error:"+err.Error())
			return
		}
		_, _ = fmt.Fprintln(os.Stdout, "done")
		return
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sessions", "rollout-session.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, acquired, err := (SessionLocker{}).TryLockCodexSessionsForMaintenance(root)
	if err != nil || !acquired {
		t.Fatalf("parent maintenance lock = %t, %v", acquired, err)
	}
	defer release()

	command := exec.Command(os.Args[0], "-test.run=^TestStateDBMaintenanceSkipsAcrossProcessesWhileChildOwnsSessionLock$")
	command.Env = append(os.Environ(), stateDBMaintenanceHelperMode+"=1", "GODEX_TEST_STATE_DB_HOME="+root)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	line, readErr := bufio.NewReader(output).ReadString('\n')
	rest, restErr := io.ReadAll(output)
	waitErr := command.Wait()
	if readErr != nil || restErr != nil || waitErr != nil || strings.TrimSpace(line) != "done" {
		t.Fatalf("maintenance helper = %q%q, read=%v rest=%v wait=%v", line, rest, readErr, restErr, waitErr)
	}
	if _, err := os.Stat(filepath.Join(root, "cache", sessionMaintenanceCacheFile)); !os.IsNotExist(err) {
		t.Fatalf("contended maintenance wrote cache: %v", err)
	}
}

func openStateDBTest(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	return database
}
