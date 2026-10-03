package codex

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const sqliteHeader = "SQLite format 3\x00"

func persistSessionGoalAttachmentPaths(codexHome string) error {
	info, err := os.Stat(codexHome)
	if err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(codexHome)
	if err != nil {
		return fmt.Errorf("read goal attachment directory %s: %w", codexHome, err)
	}
	for _, entry := range entries {
		if !sessionGoalDatabaseName(entry.Name()) {
			continue
		}
		if err := persistSessionGoalAttachmentPathsInDB(codexHome, filepath.Join(codexHome, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func persistSessionGoalAttachmentPathsInDB(codexHome, dbPath string) error {
	if !sessionGoalDatabaseLooksSQLite(dbPath) {
		return nil
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil
	}
	defer database.Close()
	hasTable, err := sessionGoalTableExists(database)
	if err != nil || !hasTable {
		return nil
	}
	rows, err := database.Query("SELECT thread_id, objective FROM thread_goals")
	if err != nil {
		return nil
	}
	type update struct {
		threadID  string
		objective string
	}
	var updates []update
	for rows.Next() {
		var threadID, objective string
		if err := rows.Scan(&threadID, &objective); err != nil {
			continue
		}
		rewritten, err := rewriteSessionPersistedAttachmentPaths(codexHome, objective)
		if err != nil {
			_ = rows.Close()
			return err
		}
		if rewritten != objective {
			updates = append(updates, update{threadID: threadID, objective: rewritten})
		}
	}
	_ = rows.Close()

	for _, item := range updates {
		if _, err := database.Exec(
			"UPDATE thread_goals SET objective = ?1 WHERE thread_id = ?2",
			item.objective, item.threadID,
		); err != nil {
			return fmt.Errorf("update goal attachments in %s: %w", dbPath, err)
		}
	}
	return nil
}

func persistSessionGoalAttachmentPathForThread(codexHome, threadID string) error {
	entries, err := os.ReadDir(codexHome)
	if err != nil {
		return fmt.Errorf("read goal attachment directory %s: %w", codexHome, err)
	}
	for _, entry := range entries {
		if !sessionGoalDatabaseName(entry.Name()) {
			continue
		}
		if err := persistSessionGoalAttachmentPathForThreadInDB(
			codexHome, filepath.Join(codexHome, entry.Name()), threadID,
		); err != nil {
			return err
		}
	}
	return nil
}

func persistSessionGoalAttachmentPathForThreadInDB(codexHome, dbPath, threadID string) error {
	if !sessionGoalDatabaseLooksSQLite(dbPath) {
		return nil
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open goal database %s: %w", dbPath, err)
	}
	defer database.Close()
	hasTable, err := sessionGoalTableExists(database)
	if err != nil {
		return fmt.Errorf("inspect goal database %s: %w", dbPath, err)
	}
	if !hasTable {
		return nil
	}
	var objective string
	err = database.QueryRow(
		"SELECT objective FROM thread_goals WHERE thread_id = ?1",
		threadID,
	).Scan(&objective)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read goal %s from %s: %w", threadID, dbPath, err)
	}
	rewritten, err := rewriteSessionPersistedAttachmentPaths(codexHome, objective)
	if err != nil {
		return err
	}
	if rewritten == objective {
		return nil
	}
	if _, err := database.Exec(
		"UPDATE thread_goals SET objective = ?1 WHERE thread_id = ?2",
		rewritten, threadID,
	); err != nil {
		return fmt.Errorf("update goal attachments in %s: %w", dbPath, err)
	}
	return nil
}

func sessionGoalDatabaseName(name string) bool {
	return strings.HasPrefix(name, "goals_") && strings.HasSuffix(name, ".sqlite")
}

func sessionGoalDatabaseLooksSQLite(path string) bool {
	file, _, err := openSessionRegularFileNoFollow(path)
	if err != nil {
		return false
	}
	defer file.Close()
	var header [16]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return false
	}
	return string(header[:]) == sqliteHeader
}

func sessionGoalTableExists(database *sql.DB) (bool, error) {
	var one int
	err := database.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'thread_goals' LIMIT 1",
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func sessionThreadID(contents string) (string, bool) {
	paths := [][]string{
		{"payload", "thread_id"},
		{"payload", "threadId"},
		{"thread_id"},
		{"threadId"},
	}
	for _, line := range strings.Split(contents, "\n") {
		var value map[string]any
		if json.Unmarshal([]byte(line), &value) != nil {
			continue
		}
		for _, path := range paths {
			current := any(value)
			for _, key := range path {
				object, ok := current.(map[string]any)
				if !ok {
					current = nil
					break
				}
				current = object[key]
			}
			text, ok := current.(string)
			if !ok {
				continue
			}
			if text = strings.TrimSpace(text); text != "" {
				return text, true
			}
		}
	}
	return "", false
}
