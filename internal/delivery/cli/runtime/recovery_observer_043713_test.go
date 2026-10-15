package runtime

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimerepo "github.com/christiandoxa/godex/internal/repository/runtime"
	_ "modernc.org/sqlite"
)

func TestProdex043713UnavailableGoalDatabaseDoesNotConsumeTerminalFailure(t *testing.T) {
	root := t.TempDir()
	session := recovery04378Session
	rollout := filepath.Join(root, "rollout-"+session+".jsonl")
	if err := os.WriteFile(rollout, []byte(`{"type":"session_meta","payload":{"id":"`+session+`"}}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(session+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := runtimerepo.NewGoalStateStore(root)
	monitor := &liveGoalRecovery04361{
		marker: marker, read: store.ReadGoalRecoveryState, observe: store.ObserveGoalRecoveryState,
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: session, Path: rollout}, nil
		},
	}
	if _, found := monitor.readState(t.Context()); found || !monitor.checkpoint.valid {
		t.Fatal("new goal DB is optional but baseline rollout checkpoint must be retained")
	}
	dbPath := filepath.Join(root, "goals_1.sqlite")
	if err := os.Mkdir(dbPath, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(rollout, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	originalID, offset, accepted := monitor.sessionID, monitor.scanOffset, monitor.accepted
	if _, found := monitor.readState(t.Context()); found {
		t.Fatal("invalid/unavailable goal DB incorrectly authorized child cancellation")
	}
	if monitor.sessionID != originalID || monitor.scanOffset != offset || monitor.accepted != accepted || monitor.observerRetryAt.IsZero() {
		t.Fatalf("observer failure consumed accepted evidence: %+v", monitor)
	}
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE thread_goals(thread_id TEXT PRIMARY KEY,status TEXT,updated_at_ms INTEGER); INSERT INTO thread_goals VALUES (?, 'blocked', 1)", session); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, found := monitor.readState(t.Context()); found {
		t.Fatal("observer retried before bounded backoff")
	}
	monitor.observerRetryAt = time.Now().Add(-time.Second)
	state, found := monitor.readState(t.Context())
	if !found || state.Status != "usage_limited" || state.SessionID != session || monitor.scanOffset == offset || monitor.pendingClass != "overload" {
		t.Fatalf("unread native terminal evidence not recovered after DB became available: %+v found=%t cursor=%d", state, found, monitor.scanOffset)
	}
	if monitor.observerRetryAt.After(time.Now()) {
		t.Fatal("successful observation retained resource retry backoff")
	}
}
