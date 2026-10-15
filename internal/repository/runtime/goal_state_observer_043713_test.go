package runtime

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestProdex043713GoalStoreDistinguishesAbsentRowFromFailedObservation(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "goals_1.sqlite")
	store := NewGoalStateStore(home)
	if _, found, ready := store.ObserveGoalRecoveryState(t.Context(), "synthetic-session"); found || !ready {
		t.Fatal("absent optional SQLite goal store must not be treated as resource pressure")
	}
	if err := os.Mkdir(dbPath, 0700); err != nil {
		t.Fatal(err)
	}
	if _, found, ready := store.ObserveGoalRecoveryState(t.Context(), "synthetic-session"); found || ready {
		t.Fatal("unreadable goal database directory must fail closed, not mean no goal")
	}
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("invalid sqlite database"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, found, ready := store.ObserveGoalRecoveryState(t.Context(), "synthetic-session"); found || ready {
		t.Fatal("corrupt goal database must not authorize reading rollout failures")
	}
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE thread_goals(thread_id TEXT PRIMARY KEY,status TEXT,updated_at_ms INTEGER); INSERT INTO thread_goals VALUES ('synthetic-session','blocked',3)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, found, ready := store.ObserveGoalRecoveryState(t.Context(), "not-present"); found || !ready {
		t.Fatal("valid goal DB without a row must be distinguishable from IO failure")
	}
	if state, found, ready := store.ObserveGoalRecoveryState(t.Context(), "synthetic-session"); !found || !ready || state.Status != "blocked" || state.UpdatedAt != 3 {
		t.Fatalf("recovered available goal state: %+v found=%t ready=%t", state, found, ready)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, found, ready := store.ObserveGoalRecoveryState(ctx, "synthetic-session"); found || ready {
		t.Fatal("cancelled goal state observation must not authorize work")
	}
}
