package runtime

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

// GoalStateStore reads Codex-owned goal state without mutating the database.
type GoalStateStore struct {
	path string
	mu   sync.Mutex
	db   *sql.DB
}

func NewGoalStateStore(sharedHome string) *GoalStateStore {
	sharedHome = strings.TrimSpace(sharedHome)
	if sharedHome == "" {
		return &GoalStateStore{}
	}
	return &GoalStateStore{path: filepath.Join(sharedHome, "goals_1.sqlite")}
}

func (store *GoalStateStore) ReadGoalRecoveryState(
	ctx context.Context, sessionID string,
) (runtimemodel.GoalRecoveryState, bool) {
	if store == nil || store.path == "" || !filepath.IsAbs(store.path) ||
		strings.TrimSpace(sessionID) == "" || ctx.Err() != nil {
		return runtimemodel.GoalRecoveryState{}, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.db == nil {
		info, err := os.Lstat(store.path)
		if err != nil || !info.Mode().IsRegular() {
			return runtimemodel.GoalRecoveryState{}, false
		}
		db, err := sql.Open("sqlite", goalReadOnlyDSN(store.path, goruntime.GOOS))
		if err != nil {
			return runtimemodel.GoalRecoveryState{}, false
		}
		db.SetMaxOpenConns(1)
		store.db = db
	}
	var state runtimemodel.GoalRecoveryState
	err := store.db.QueryRowContext(ctx,
		"SELECT status,updated_at_ms FROM thread_goals WHERE thread_id = ? ORDER BY updated_at_ms DESC LIMIT 1", sessionID,
	).Scan(&state.Status, &state.UpdatedAt)
	if err != nil {
		return runtimemodel.GoalRecoveryState{}, false
	}
	state.SessionID = sessionID
	return state, true
}

func (store *GoalStateStore) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.db == nil {
		return nil
	}
	err := store.db.Close()
	store.db = nil
	return err
}

func goalReadOnlyDSN(path, goos string) string {
	normalized := filepath.ToSlash(path)
	if goos == "windows" {
		normalized = strings.ReplaceAll(path, "\\", "/")
		if len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/' &&
			((normalized[0] >= 'A' && normalized[0] <= 'Z') ||
				(normalized[0] >= 'a' && normalized[0] <= 'z')) {
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
