package runtime

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestProdex04360GoalDatabaseReadOnlyDSNPortable(t *testing.T) {
	for _, tc := range []struct {
		name, path, goos, want string
	}{
		{"windows_drive", `C:\Users\Runner One\goals_1.sqlite`, "windows", "/C:/Users/Runner One/goals_1.sqlite"},
		{"windows_lower_drive", `d:\state\goals_1.sqlite`, "windows", "/d:/state/goals_1.sqlite"},
		{"unix", "/tmp/Test Home/goals_1.sqlite", "linux", "/tmp/Test Home/goals_1.sqlite"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := goalReadOnlyDSN04360(tc.path, tc.goos)
			uri, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			if uri.Scheme != "file" || uri.Host != "" || uri.Path != tc.want {
				t.Fatalf("bad portable SQLite DSN %q => %#v", dsn, uri)
			}
			query := uri.Query()
			if query.Get("mode") != "ro" ||
				!containsGoalPragma04360(query["_pragma"], "query_only(1)") ||
				!containsGoalPragma04360(query["_pragma"], "busy_timeout(2000)") {
				t.Fatalf("read-only SQLite options missing: %q", dsn)
			}
		})
	}
}
func containsGoalPragma04360(pragma []string, needle string) bool {
	for _, item := range pragma {
		if item == needle {
			return true
		}
	}
	return false
}
func TestProdex04360GoalDatabaseConnectionCannotMutateState(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	root := t.TempDir()
	path := fixtureGoalDB04360(t, root, session, "active")
	dsn := goalReadOnlyDSN04360(path, runtime.GOOS)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var goal string
	if err := db.QueryRowContext(context.Background(), "SELECT status FROM thread_goals WHERE thread_id=?", session).Scan(&goal); err != nil || goal != "active" {
		t.Fatalf("goal query failed %q err %v", goal, err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE thread_goals SET status='completed' WHERE thread_id=?", session); err == nil {
		t.Fatal("read-only recovery goal database accepted a mutation")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || strings.TrimSpace(goal) != "active" {
		t.Fatalf("database mutated unexpectedly: len %d goal %q", len(raw), goal)
	}
}
