package runtime

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func fixtureGoalDB04360(t *testing.T, root, id, status string) string {
	t.Helper()
	path := filepath.Join(root, "goals_1.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec("CREATE TABLE thread_goals (thread_id TEXT, status TEXT, updated_at_ms INTEGER)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO thread_goals (thread_id,status,updated_at_ms) VALUES (?,?,?)", id, status, int64(1899999999000))
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func TestProdex04360GoalDatabaseStatusGatesRecovery(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{"active", true}, {"paused", true}, {"blocked", true}, {"usage_limited", true},
		{"completed", false}, {"cancelled", false}, {"failed", false},
	} {
		t.Run(tc.status, func(t *testing.T) {
			home := t.TempDir()
			fixtureGoalDB04360(t, home, session, tc.status)
			if got := goalAllowsRecovery04360(t.Context(), home, session); got != tc.want {
				t.Fatalf("goal status %q allows recovery=%t want=%t", tc.status, got, tc.want)
			}
		})
	}
}

func TestProdex04360GoalDatabaseMissingOrUnsafeFailsClosedWhenPresent(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	home := t.TempDir()
	if !goalAllowsRecovery04360(t.Context(), home, session) {
		t.Fatal("optional absent goal database blocked non-goal session")
	}
	external := t.TempDir()
	target := fixtureGoalDB04360(t, external, session, "active")
	if err := os.Symlink(target, filepath.Join(home, "goals_1.sqlite")); err == nil {
		if goalAllowsRecovery04360(t.Context(), home, session) {
			t.Fatal("symlink goal DB trusted")
		}
	}
	if err := os.Remove(filepath.Join(home, "goals_1.sqlite")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "goals_1.sqlite"), []byte("not a sqlite db"), 0600); err != nil {
		t.Fatal(err)
	}
	if goalAllowsRecovery04360(t.Context(), home, session) {
		t.Fatal("malformed goal DB authorized continuation")
	}
}
