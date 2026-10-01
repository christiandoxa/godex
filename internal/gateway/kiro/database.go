package kiro

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

var kiroAuthKeyPriority = []string{
	"kirocli:social:token",
	"kirocli:external-idp:token",
	"codewhisperer:odic:token",
}

const (
	kiroProfileStateKey  = "api.codewhisperer.profile"
	kiroStartURLStateKey = "auth.idc.start-url"
	kiroRegionStateKey   = "auth.idc.region"
)

type kiroProfileState struct {
	ARN         string `json:"arn"`
	ProfileName string `json:"profile_name"`
	UserID      string `json:"user_id"`
}

func readSourceDatabase(ctx context.Context, path string) (sourceSnapshot, error) {
	if err := ensureRegularDatabase(path); err != nil {
		return sourceSnapshot{}, err
	}
	database, err := sql.Open("sqlite", readOnlySQLiteDSN(path))
	if err != nil {
		return sourceSnapshot{}, fmt.Errorf("open Kiro auth database: %w", err)
	}
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		return sourceSnapshot{}, fmt.Errorf("open Kiro auth database: %w", err)
	}
	authKey, authJSON, err := readKiroAuthToken(ctx, database)
	if err != nil {
		return sourceSnapshot{}, err
	}
	profile, err := readKiroProfileState(ctx, database)
	if err != nil {
		return sourceSnapshot{}, err
	}
	startURL, err := readKiroStateValue(ctx, database, kiroStartURLStateKey)
	if err != nil {
		return sourceSnapshot{}, err
	}
	region, err := readKiroStateValue(ctx, database, kiroRegionStateKey)
	if err != nil {
		return sourceSnapshot{}, err
	}
	return sourceSnapshot{
		AuthKey: authKey, AuthJSON: authJSON, ProfileARN: profile.ARN,
		ProfileName: profile.ProfileName, UserID: profile.UserID,
		StartURL: startURL, Region: region,
	}, nil
}

func readOnlySQLiteDSN(path string) string {
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", "busy_timeout(2000)")
	uri.RawQuery = query.Encode()
	return uri.String()
}

func readKiroAuthToken(ctx context.Context, database *sql.DB) (string, string, error) {
	for _, key := range kiroAuthKeyPriority {
		value, err := queryOptionalString(ctx, database, "SELECT value FROM auth_kv WHERE key = ?1", key)
		if err != nil {
			return "", "", fmt.Errorf("read Kiro auth token: %w", err)
		}
		if value = strings.TrimSpace(value); value != "" {
			return key, value, nil
		}
	}
	var key, value string
	err := database.QueryRowContext(ctx, "SELECT key, value FROM auth_kv WHERE key LIKE '%:token' AND trim(value) != '' ORDER BY key LIMIT 1").Scan(&key, &value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", errors.New("no logged-in Kiro credential found in auth_kv")
	}
	if err != nil {
		return "", "", fmt.Errorf("read Kiro auth token: %w", err)
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), nil
}

func readKiroProfileState(ctx context.Context, database *sql.DB) (kiroProfileState, error) {
	value, err := readKiroStateValue(ctx, database, kiroProfileStateKey)
	if err != nil || value == "" {
		return kiroProfileState{}, err
	}
	var profile kiroProfileState
	if err := json.Unmarshal([]byte(value), &profile); err != nil {
		return kiroProfileState{}, fmt.Errorf("failed to parse Kiro state key %q", kiroProfileStateKey)
	}
	return profile, nil
}

func readKiroStateValue(ctx context.Context, database *sql.DB, key string) (string, error) {
	value, err := queryOptionalString(ctx, database, "SELECT value FROM state WHERE key = ?1", key)
	if err != nil {
		return "", fmt.Errorf("read Kiro state key %q: %w", key, err)
	}
	return strings.TrimSpace(value), nil
}

func queryOptionalString(ctx context.Context, database *sql.DB, query string, arguments ...any) (string, error) {
	var value string
	err := database.QueryRowContext(ctx, query, arguments...).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}
