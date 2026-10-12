package runtime

import (
	"context"
	"database/sql"
	"path/filepath"
	goruntime "runtime"
	"testing"

	_ "modernc.org/sqlite"
)

func TestGoalStateStoreReadsLatestStateReadOnly(t *testing.T) {
	home := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(home, "goals_1.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`CREATE TABLE thread_goals (thread_id TEXT, status TEXT, updated_at_ms INTEGER);
INSERT INTO thread_goals(thread_id,status,updated_at_ms) VALUES ('session','active',1),('session','usage_limited',2);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store := NewGoalStateStore(home)
	defer store.Close()
	state, ok := store.ReadGoalRecoveryState(context.Background(), "session")
	if !ok || state.SessionID != "session" || state.Status != "usage_limited" || state.UpdatedAt != 2 {
		t.Fatalf("state=%+v ok=%t", state, ok)
	}
	writer, err := sql.Open("sqlite", filepath.Join(home, "goals_1.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.Exec("UPDATE thread_goals SET status='complete'"); err != nil {
		t.Fatalf("writer blocked after read-only goal state read: %v", err)
	}
	readOnly, err := sql.Open("sqlite", goalReadOnlyDSN(filepath.Join(home, "goals_1.sqlite"), goruntime.GOOS))
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	if _, err := readOnly.Exec("UPDATE thread_goals SET status='active'"); err == nil {
		t.Fatal("read-only goal state accepted a mutation")
	}
}
