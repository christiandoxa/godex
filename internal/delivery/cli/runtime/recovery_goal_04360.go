package runtime

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	_ "modernc.org/sqlite"
)

type goalRecoveryState04360 struct {
	present   bool
	status    string
	updatedAt int64
}

func readGoalRecoveryState04360(ctx context.Context, home, id string) (goalRecoveryState04360, bool) {
	if ctx.Err() != nil {
		return goalRecoveryState04360{}, false
	}
	if strings.TrimSpace(home) == "" {
		return goalRecoveryState04360{}, true
	}
	path := filepath.Join(home, "goals_1.sqlite")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return goalRecoveryState04360{}, true
	}
	if err != nil || !info.Mode().IsRegular() {
		return goalRecoveryState04360{}, false
	}
	// Windows filepath.Join uses backslashes and a drive letter. A raw
	// file URL constructed from that string is not a valid SQLite URI on
	// Windows. Use the same normalized, query-only URI contract as the
	// repository's Kiro SQLite read-only adapter.
	database, err := sql.Open("sqlite", goalReadOnlyDSN04360(path, goruntime.GOOS))
	if err != nil {
		return goalRecoveryState04360{}, false
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	var present int
	err = database.QueryRowContext(ctx,
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='thread_goals' LIMIT 1",
	).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return goalRecoveryState04360{}, true
	}
	if err != nil {
		return goalRecoveryState04360{}, false
	}
	var result goalRecoveryState04360
	err = database.QueryRowContext(ctx,
		"SELECT status,updated_at_ms FROM thread_goals WHERE thread_id = ? ORDER BY updated_at_ms DESC LIMIT 1", id,
	).Scan(&result.status, &result.updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return goalRecoveryState04360{}, true
	}
	if err != nil {
		return goalRecoveryState04360{}, false
	}
	result.present = true
	result.status = strings.ToLower(strings.TrimSpace(result.status))
	return result, true
}

type goalTransition04360 struct {
	home   string
	id     string
	before goalRecoveryState04360
	valid  bool
}

func captureGoalTransition04360(ctx context.Context, home, id string) goalTransition04360 {
	prior, ok := readGoalRecoveryState04360(ctx, home, id)
	valid := ok && prior.present
	if valid {
		switch prior.status {
		case "active", "paused", "blocked":
		default:
			valid = false
		}
	}
	return goalTransition04360{home: home, id: id, before: prior, valid: valid}
}

// The tagged monitor arms on a previously resumable non-terminal goal, and
// observes a fresh transition to usage_limited; a historical, unchanged
// terminal state cannot trigger another child invocation.
func (checkpoint goalTransition04360) newUsageLimit04360(ctx context.Context) bool {
	if !checkpoint.valid || ctx.Err() != nil {
		return false
	}
	next, ok := readGoalRecoveryState04360(ctx, checkpoint.home, checkpoint.id)
	return ok && next.present && next.status == "usage_limited" && next.updatedAt > checkpoint.before.updatedAt
}

func goalAllowsRecovery04360(ctx context.Context, home, id string) bool {
	status, ok := readGoalRecoveryState04360(ctx, home, id)
	if !ok {
		return false
	}
	if !status.present {
		return true
	}
	switch status.status {
	case "active", "paused", "blocked", "usage_limited":
		return true
	default:
		return false
	}
}

// goalReadOnlyDSN04360 preserves Windows drive semantics and enforces a
// query-only connection. Constructing a file URL from a raw C:\ path
// would cause a false-negative goal state probe on Windows CI.
func goalReadOnlyDSN04360(path, goos string) string {
	normalized := filepath.ToSlash(path)
	if goos == "windows" {
		normalized = strings.ReplaceAll(path, "\\", "/")
		if len(normalized) >= 3 && normalized[1] == ':' &&
			normalized[2] == '/' &&
			(normalized[0] >= 'A' && normalized[0] <= 'Z' ||
				normalized[0] >= 'a' && normalized[0] <= 'z') {
			normalized = "/" + normalized
		}
	}
	uri := url.URL{Scheme: "file", Path: normalized}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", "busy_timeout(2000)")
	uri.RawQuery = query.Encode()
	return uri.String()
}
