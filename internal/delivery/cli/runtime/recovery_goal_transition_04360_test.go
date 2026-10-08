package runtime

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestProdex04360ActiveGoalTransitionDetectsFreshUsageLimit(t *testing.T) {
	const id = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		before, after     string
		beforeAt, afterAt int64
		want              bool
	}{
		{"active", "usage_limited", 100, 101, true},
		{"active", "usage_limited", 100, 100, false},
		{"paused", "usage_limited", 100, 101, false},
		{"active", "completed", 100, 101, false},
		{"usage_limited", "usage_limited", 100, 100, false},
	} {
		t.Run(tc.before+"-to-"+tc.after, func(t *testing.T) {
			root := t.TempDir()
			path := fixtureGoalDB04360(t, root, id, tc.before)
			database, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = database.Exec("UPDATE thread_goals SET updated_at_ms = ?", tc.beforeAt); err != nil {
				t.Fatal(err)
			}
			baseline := captureGoalTransition04360(t.Context(), root, id)
			if _, err = database.Exec("UPDATE thread_goals SET status=?,updated_at_ms=?", tc.after, tc.afterAt); err != nil {
				t.Fatal(err)
			}
			if err = database.Close(); err != nil {
				t.Fatal(err)
			}
			got := baseline.newUsageLimit04360(context.Background())
			if got != tc.want {
				t.Fatalf("goal transition %s->%s at %d->%d = %t want %t", tc.before, tc.after, tc.beforeAt, tc.afterAt, got, tc.want)
			}
		})
	}
	missing := captureGoalTransition04360(context.Background(), filepath.Join(t.TempDir(), "not-present"), id)
	if missing.newUsageLimit04360(context.Background()) {
		t.Fatal("missing DB fabricated a goal limit")
	}
}
