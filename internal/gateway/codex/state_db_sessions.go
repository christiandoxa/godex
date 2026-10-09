package codex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	maxStateDBFiles = 64
	maxStateDBRows  = 8192
	maxStateDBScan  = 4096
	maxStateDBText  = 4096
)

type stateDBThread struct {
	id          string
	rolloutPath string
}

type stateDBSessionCandidate struct {
	path     string
	selector string
}

func stateDBSessionCandidates(root string) ([]stateDBSessionCandidate, error) {
	if err := validateCodexHomePath(root); err != nil {
		return nil, err
	}
	databases, err := stateDBFiles(root)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]stateDBSessionCandidate)
	for _, database := range databases {
		rows, err := readStateDBThreads(database)
		if err != nil {
			continue
		}
		for _, row := range rows {
			path, ok := resolveStateDBRolloutPath(root, row.rolloutPath)
			if !ok {
				continue
			}
			candidate := stateDBSessionCandidate{path: path, selector: stateDBThreadID(row)}
			if previous, exists := byPath[path]; !exists || previous.selector == "" {
				byPath[path] = candidate
			}
		}
	}
	result := make([]stateDBSessionCandidate, 0, len(byPath))
	for _, candidate := range byPath {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].path < result[j].path })
	return result, nil
}

// repairStateDBRolloutPaths updates only stale overlay rows whose replacement
// is a verified rollout below the shared Codex root. The expected old path is
// part of the UPDATE so a concurrent Codex write is never overwritten.
func repairStateDBRolloutPaths(root string) error {
	if err := validateCodexHomePath(root); err != nil {
		return err
	}
	persistent, err := persistentStateDBRollouts(root)
	if err != nil {
		return err
	}
	databases, err := stateDBFiles(root)
	if err != nil {
		return err
	}
	for _, database := range databases {
		rows, readErr := readStateDBThreads(database)
		if readErr != nil {
			continue
		}
		for _, row := range rows {
			if !strings.Contains(row.rolloutPath, ".prodex-overlay-") {
				continue
			}
			id := stateDBThreadID(row)
			replacement := persistent[id]
			if replacement == "" || replacement == row.rolloutPath {
				continue
			}
			if err := replaceStateDBRolloutPath(database, row, replacement); err != nil {
				return err
			}
		}
	}
	return nil
}

func stateDBFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read Codex state root: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "state_") || !strings.HasSuffix(name, ".sqlite") {
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		paths = append(paths, path)
		if len(paths) > maxStateDBFiles {
			return nil, errors.New("Codex state database count exceeds safe limit")
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func readStateDBThreads(path string) ([]stateDBThread, error) {
	database, err := sql.Open("sqlite", stateDBReadOnlyURI(path))
	if err != nil {
		return nil, err
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	if err := database.Ping(); err != nil {
		return nil, err
	}
	rows, err := database.QueryContext(context.Background(), "SELECT id, rollout_path FROM threads LIMIT ?", maxStateDBRows+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	threads := make([]stateDBThread, 0)
	for rows.Next() {
		var thread stateDBThread
		if err := rows.Scan(&thread.id, &thread.rolloutPath); err != nil {
			return nil, err
		}
		if len(thread.id) > maxStateDBText || len(thread.rolloutPath) > maxStateDBText {
			return nil, errors.New("Codex state database thread row exceeds safe size")
		}
		threads = append(threads, thread)
		if len(threads) > maxStateDBRows {
			return nil, errors.New("Codex state database thread count exceeds safe limit")
		}
	}
	return threads, rows.Err()
}

func replaceStateDBRolloutPath(path string, row stateDBThread, replacement string) error {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	if _, err := database.Exec("PRAGMA busy_timeout = 3000"); err != nil {
		return err
	}
	result, err := database.Exec(
		"UPDATE threads SET rollout_path = ?1 WHERE id = ?2 AND rollout_path = ?3",
		replacement, row.id, row.rolloutPath,
	)
	if err != nil {
		return fmt.Errorf("repair Codex state database %s: %w", path, err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return err
	}
	return nil
}

func persistentStateDBRollouts(root string) (map[string]string, error) {
	result := make(map[string]string)
	for _, directory := range []string{"sessions", "archived_sessions"} {
		base := filepath.Join(root, directory)
		if err := collectPersistentStateDBRollouts(base, root, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func collectPersistentStateDBRollouts(directory, root string, result map[string]string) error {
	if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	count := 0
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil || entry.Type()&os.ModeSymlink != 0 {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() || !isStateDBRolloutName(entry.Name()) {
			return nil
		}
		count++
		if count > maxStateDBScan {
			return errors.New("Codex rollout count exceeds safe limit")
		}
		id, ok := sessionIDFromPath(path)
		if !ok {
			id = sessionIDFromRollout(path)
			ok = id != ""
		}
		if !ok {
			return nil
		}
		if contained, ok := resolveStateDBRolloutPath(root, path); ok {
			key := strings.ToLower(id)
			if _, exists := result[key]; !exists {
				result[key] = contained
			}
		}
		return nil
	})
	return err
}

func stateDBThreadID(row stateDBThread) string {
	id := strings.TrimPrefix(strings.TrimSpace(row.id), "thread_")
	if !fullSessionID(id) {
		if fallback, ok := sessionIDFromPath(row.rolloutPath); ok {
			id = fallback
		}
	}
	return strings.ToLower(id)
}

func sessionIDFromRollout(path string) string {
	contents, found, err := readSessionAttachmentFile(path)
	if err != nil || !found {
		return ""
	}
	for _, line := range strings.Split(contents, "\n") {
		id := sessionLineResumeID(line)
		if fullSessionID(id) {
			return id
		}
	}
	return ""
}

func resolveStateDBRolloutPath(root, value string) (string, bool) {
	path := filepath.Clean(value)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", false
	}
	if !sessionPathContained(root, path) {
		return "", false
	}
	if !isStateDBRolloutName(filepath.Base(path)) {
		return "", false
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false
	}
	return path, true
}

func isStateDBRolloutName(name string) bool {
	return strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl.zst")
}

func sessionPathContained(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return false
	}
	current := root
	for _, component := range strings.Split(relative, string(os.PathSeparator)) {
		if component == "" || component == "." || component == ".." {
			return false
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

func stateDBReadOnlyURI(path string) string {
	return stateDBReadOnlyURIForOS(path, runtime.GOOS)
}

// stateDBReadOnlyURIForOS preserves drive paths as file:///C:/... on Windows.
// A raw drive path in url.URL.Path otherwise parses as a file URL with a host,
// causing state database probes and verified rollout repairs to be skipped.
func stateDBReadOnlyURIForOS(path, goos string) string {
	normalized := filepath.ToSlash(path)
	if goos == "windows" {
		normalized = strings.ReplaceAll(path, "\\", "/")
		if len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/' &&
			(normalized[0] >= 'A' && normalized[0] <= 'Z' ||
				normalized[0] >= 'a' && normalized[0] <= 'z') {
			normalized = "/" + normalized
		}
	}
	value := url.URL{Scheme: "file", Path: normalized}
	value.RawQuery = "mode=ro"
	return value.String()
}
