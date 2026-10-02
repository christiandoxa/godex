package kiro

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
	_ "modernc.org/sqlite"
)

const runtimeDataDirName = "kiro-data"

type runtimeCredential struct {
	dataDir string
	secret  authSecret
}

func (source *Source) prepareRuntimeCredential(ctx context.Context, home string) (runtimeCredential, error) {
	text, found, err := readManagedQuotaFile(home, CredentialsFile)
	if err != nil {
		return runtimeCredential{}, err
	}
	if !found {
		return runtimeCredential{}, errors.New("Kiro runtime requires an imported auth snapshot")
	}
	var secret authSecret
	if err := json.Unmarshal([]byte(text), &secret); err != nil {
		return runtimeCredential{}, errors.New("failed to parse Kiro runtime auth snapshot")
	}
	if _, err := source.InspectAuthSecret(ctx, text); err != nil {
		return runtimeCredential{}, err
	}
	dataDir := filepath.Join(filepath.Clean(home), runtimeDataDirName)
	if err := ensurePrivateRuntimeDir(dataDir); err != nil {
		return runtimeCredential{}, err
	}
	stored, found, err := readRuntimeDatabaseAuth(ctx, dataDir, secret.AuthKey)
	if err != nil {
		return runtimeCredential{}, err
	}
	switch {
	case !found:
		if err := writeRuntimeDatabase(ctx, dataDir, secret); err != nil {
			return runtimeCredential{}, err
		}
	case newerKiroAuth(secret.AuthJSON, stored):
		if err := writeRuntimeDatabase(ctx, dataDir, secret); err != nil {
			return runtimeCredential{}, err
		}
	case secret.AuthJSON != stored:
		secret.AuthJSON = stored
		if err := writeManagedRuntimeSecret(home, secret); err != nil {
			return runtimeCredential{}, err
		}
	}
	return runtimeCredential{dataDir: dataDir, secret: secret}, nil
}

func ensurePrivateRuntimeDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create Kiro runtime data directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return errors.New("inspect Kiro runtime data directory")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Kiro runtime data directory must be a real directory")
	}
	return os.Chmod(path, 0o700)
}

func readRuntimeDatabaseAuth(ctx context.Context, dataDir, authKey string) (string, bool, error) {
	path := filepath.Join(dataDir, kiroDatabaseFileName)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("inspect Kiro runtime database")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false, errors.New("Kiro runtime database must be a regular file")
	}
	database, err := sql.Open("sqlite", readOnlySQLiteDSN(path))
	if err != nil {
		return "", false, errors.New("open Kiro runtime database")
	}
	defer database.Close()
	var exists bool
	if err := database.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE type='table' AND name='auth_kv')").Scan(&exists); err != nil {
		return "", false, errors.New("inspect Kiro runtime auth table")
	}
	if !exists {
		return "", false, nil
	}
	value, err := queryOptionalString(ctx, database, "SELECT value FROM auth_kv WHERE key = ?1", authKey)
	if err != nil {
		return "", false, errors.New("read Kiro runtime auth")
	}
	return value, strings.TrimSpace(value) != "", nil
}

func writeRuntimeDatabase(ctx context.Context, dataDir string, secret authSecret) error {
	path := filepath.Join(dataDir, kiroDatabaseFileName)
	if err := validateRuntimeDatabasePath(path); err != nil {
		return err
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return errors.New("open Kiro runtime database")
	}
	defer database.Close()
	if err := initializeRuntimeDatabase(ctx, database); err != nil {
		return err
	}
	if err := writeRuntimeAuth(ctx, database, secret); err != nil {
		return err
	}
	if err := writeRuntimeProfileState(ctx, database, secret); err != nil {
		return err
	}
	if err := writeRuntimeOptionalState(ctx, database, kiroStartURLStateKey, secret.StartURL); err != nil {
		return err
	}
	if err := writeRuntimeOptionalState(ctx, database, kiroRegionStateKey, secret.Region); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return errors.New("secure Kiro runtime database")
	}
	return nil
}

func validateRuntimeDatabasePath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("inspect Kiro runtime database")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("Kiro runtime database must be a regular file")
	}
	return nil
}

func initializeRuntimeDatabase(ctx context.Context, database *sql.DB) error {
	if _, err := database.ExecContext(ctx, runtimeDatabaseSchema); err != nil {
		return errors.New("initialize Kiro runtime database")
	}
	for version := 0; version <= 9; version++ {
		if _, err := database.ExecContext(ctx, "INSERT OR IGNORE INTO migrations(version,migration_time) VALUES(?1,strftime('%s','now'))", version); err != nil {
			return errors.New("write Kiro runtime migration state")
		}
	}
	return nil
}

func writeRuntimeAuth(ctx context.Context, database *sql.DB, secret authSecret) error {
	if _, err := database.ExecContext(ctx, "DELETE FROM auth_kv WHERE key LIKE '%:token' AND key != ?1", secret.AuthKey); err != nil {
		return errors.New("replace Kiro runtime auth")
	}
	if _, err := database.ExecContext(ctx, "INSERT OR REPLACE INTO auth_kv(key,value) VALUES(?1,?2)", secret.AuthKey, secret.AuthJSON); err != nil {
		return errors.New("write Kiro runtime auth")
	}
	return nil
}

func writeRuntimeProfileState(ctx context.Context, database *sql.DB, secret authSecret) error {
	profileState := runtimeProfileState(secret)
	if len(profileState) == 0 {
		_, _ = database.ExecContext(ctx, "DELETE FROM state WHERE key=?1", kiroProfileStateKey)
		return nil
	}
	content, err := json.Marshal(profileState)
	if err != nil {
		return errors.New("serialize Kiro runtime profile state")
	}
	if _, err := database.ExecContext(ctx, "INSERT OR REPLACE INTO state(key,value) VALUES(?1,?2)", kiroProfileStateKey, string(content)); err != nil {
		return errors.New("write Kiro runtime profile state")
	}
	return nil
}

func runtimeProfileState(secret authSecret) map[string]any {
	state := make(map[string]any)
	if value := optionalTrimmed(secret.ProfileARN); value != "" {
		state["arn"] = value
	}
	if value := optionalTrimmed(secret.ProfileName); value != "" {
		state["profile_name"] = value
	}
	if value := optionalTrimmed(secret.Email); value != "" {
		state["user_id"] = value
	}
	return state
}

func writeRuntimeOptionalState(ctx context.Context, database *sql.DB, key string, value *string) error {
	trimmed := optionalTrimmed(value)
	if trimmed == "" {
		_, _ = database.ExecContext(ctx, "DELETE FROM state WHERE key=?1", key)
		return nil
	}
	if _, err := database.ExecContext(ctx, "INSERT OR REPLACE INTO state(key,value) VALUES(?1,?2)", key, trimmed); err != nil {
		return errors.New("write Kiro runtime state")
	}
	return nil
}

func optionalTrimmed(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

const runtimeDatabaseSchema = `
CREATE TABLE IF NOT EXISTS migrations(id INTEGER PRIMARY KEY, version INTEGER NOT NULL, migration_time INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS history(id INTEGER PRIMARY KEY, command TEXT, shell TEXT, pid INTEGER, session_id TEXT, cwd TEXT, start_time INTEGER, end_time INTEGER, duration INTEGER, hostname TEXT, exit_code INTEGER);
CREATE TABLE IF NOT EXISTS state(key TEXT PRIMARY KEY, value BLOB);
CREATE TABLE IF NOT EXISTS auth_kv(key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE IF NOT EXISTS conversations(key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE IF NOT EXISTS conversations_v2(key TEXT NOT NULL, conversation_id TEXT NOT NULL, value TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, PRIMARY KEY(key,conversation_id));
CREATE INDEX IF NOT EXISTS idx_conversations_v2_key_updated ON conversations_v2(key,updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_conversations_v2_updated_at ON conversations_v2(updated_at DESC);
CREATE TABLE IF NOT EXISTS extracted_kas_versions(version TEXT PRIMARY KEY, last_used_at INTEGER NOT NULL);`

func newerKiroAuth(candidate, stored string) bool {
	return kiroAuthExpiry(candidate).After(kiroAuthExpiry(stored))
}

func kiroAuthExpiry(raw string) time.Time {
	var value map[string]any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return time.Time{}
	}
	for _, key := range []string{"expires_at", "expiresAt"} {
		if text, ok := value[key].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, text); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func writeManagedRuntimeSecret(home string, secret authSecret) error {
	content, err := json.MarshalIndent(secret, "", "  ")
	if err != nil {
		return errors.New("serialize refreshed Kiro auth snapshot")
	}
	path := filepath.Join(filepath.Clean(home), CredentialsFile)
	if _, err := fileutil.AtomicWrite(path, content); err != nil {
		return errors.New("persist refreshed Kiro auth snapshot")
	}
	return os.Chmod(path, 0o600)
}
