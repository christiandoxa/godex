package kiro

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestKiroSourceLoadsReadOnlyDatabaseAndNormalizedCatalog(t *testing.T) {
	databasePath := writeKiroDatabaseFixture(t, map[string]string{
		"codewhisperer:odic:token": `{"email":"fallback@example.test","region":"eu-west-1"}`,
		"kirocli:social:token":     `{"access_token":"fixture","region":"token-region"}`,
	}, map[string]string{
		kiroProfileStateKey:  `{"arn":"arn:fixture","profile_name":"upstream-main","user_id":"state-user@example.test"}`,
		kiroStartURLStateKey: "https://state.example.test/start",
		kiroRegionStateKey:   "us-east-1",
	})
	var catalogEnvironment map[string]string
	source := testKiroSource(databasePath, func(_ context.Context, _ string, arguments []string, environment map[string]string) (metadataResult, error) {
		switch strings.Join(arguments, " ") {
		case "whoami --format json":
			return metadataResult{stdout: []byte(`{"email":"whoami@example.test"}`)}, nil
		case "chat --list-models --format json":
			catalogEnvironment = cloneStringMap(environment)
			return metadataResult{stdout: []byte(`{"supportedModels":[{"model_id":"model-a","model_name":"Model A","contextWindowTokens":8192},{"modelId":"model-b"},{"model_id":"model-a"}]}`)}, nil
		default:
			t.Fatalf("unexpected Kiro command: %#v", arguments)
			return metadataResult{}, nil
		}
	})

	credential, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if credential.Warning != "" || credential.Email != "whoami@example.test" || credential.Provider.Kind != "kiro" {
		t.Fatalf("credential = %#v", credential)
	}
	assertOptionalValue(t, credential.Provider.AuthKey, "kirocli:social:token")
	assertOptionalValue(t, credential.Provider.AuthKind, "social")
	assertOptionalValue(t, credential.Provider.ProfileARN, "arn:fixture")
	assertOptionalValue(t, credential.Provider.ProfileName, "upstream-main")
	assertOptionalValue(t, credential.Provider.StartURL, "https://state.example.test/start")
	assertOptionalValue(t, credential.Provider.Region, "us-east-1")
	if len(credential.SecretFiles) != 2 || credential.SecretFiles[0].Path != CredentialsFile || credential.SecretFiles[1].Path != ModelCatalogFile {
		t.Fatalf("secret files = %#v", credential.SecretFiles)
	}
	var catalog struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal([]byte(credential.SecretFiles[1].Text), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 2 || catalog.Models[0]["id"] != "model-a" || catalog.Models[0]["owned_by"] != "kiro-cli" || catalog.Models[0]["context_window_tokens"] != float64(8192) || catalog.Models[1]["name"] != "model-b" {
		t.Fatalf("normalized catalog = %#v", catalog.Models)
	}
	if catalogEnvironment["KIRO_TEST_DB_PATH"] != databasePath || catalogEnvironment["KIRO_DATA_DIR"] != filepath.Dir(databasePath) || catalogEnvironment["Q_CLI_DATA_DIR"] != filepath.Dir(databasePath) || catalogEnvironment["AWS_REGION"] != "us-east-1" {
		t.Fatalf("catalog environment = %#v", catalogEnvironment)
	}
}

func TestKiroSourceCatalogFailureIsNonFatal(t *testing.T) {
	databasePath := writeKiroDatabaseFixture(t, map[string]string{
		"kirocli:external-idp:token": `{"access_token":"fixture","email":"person@example.test"}`,
	}, nil)
	source := testKiroSource(databasePath, func(_ context.Context, _ string, arguments []string, _ map[string]string) (metadataResult, error) {
		if arguments[0] == "whoami" {
			return metadataResult{exitCode: 1}, nil
		}
		return metadataResult{exitCode: 1}, nil
	})
	credential, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if credential.Email != "person@example.test" || credential.Warning == "" || len(credential.SecretFiles) != 1 || credential.SecretFiles[0].Path != CredentialsFile {
		t.Fatalf("credential = %#v", credential)
	}
	assertOptionalValue(t, credential.Provider.AuthKind, "external-idp")
}

func TestKiroSourceUsesFallbackTokenAndBuilderIdentity(t *testing.T) {
	databasePath := writeKiroDatabaseFixture(t, map[string]string{
		"z-custom:token": `{"access_token":"fixture"}`,
		"a-custom:token": `{"access_token":"fixture"}`,
	}, nil)
	source := testKiroSource(databasePath, func(_ context.Context, _ string, arguments []string, _ map[string]string) (metadataResult, error) {
		if arguments[0] == "whoami" {
			return metadataResult{stdout: []byte(`{"username":"not-an-email"}`)}, nil
		}
		return metadataResult{stdout: []byte(`{"models":[{"id":"model-a"}]}`)}, nil
	})
	credential, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertOptionalValue(t, credential.Provider.AuthKey, "a-custom:token")
	assertOptionalValue(t, credential.Provider.AuthKind, "builder-id")
}

func TestKiroSourceRejectsSymlinkDatabaseOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics vary on Windows")
	}
	realPath := writeKiroDatabaseFixture(t, map[string]string{"kirocli:social:token": `{"access_token":"fixture"}`}, nil)
	link := filepath.Join(t.TempDir(), "data.sqlite3")
	if err := os.Symlink(realPath, link); err != nil {
		t.Fatal(err)
	}
	source := NewSource()
	source.getenv = func(key string) string {
		if key == "KIRO_TEST_DB_PATH" {
			return link
		}
		return ""
	}
	source.homeDir = func() (string, error) { return t.TempDir(), nil }
	if _, err := source.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to find Kiro auth database") {
		t.Fatalf("symlink database error = %v", err)
	}
}

func testKiroSource(databasePath string, runner metadataRunner) *Source {
	source := NewSource()
	source.getenv = func(key string) string {
		switch key {
		case "KIRO_TEST_DB_PATH":
			return databasePath
		case "PRODEX_KIRO_BIN":
			return "kiro-fixture"
		default:
			return ""
		}
	}
	source.homeDir = func() (string, error) { return filepath.Dir(filepath.Dir(databasePath)), nil }
	source.run = runner
	return source
}

func writeKiroDatabaseFixture(t *testing.T, auth, state map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.sqlite3")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE auth_kv (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE state (key TEXT PRIMARY KEY, value BLOB);`); err != nil {
		t.Fatal(err)
	}
	for key, value := range auth {
		if _, err := database.Exec(`INSERT INTO auth_kv(key,value) VALUES(?,?)`, key, value); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range state {
		if _, err := database.Exec(`INSERT INTO state(key,value) VALUES(?,?)`, key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertOptionalValue(t *testing.T, value *string, want string) {
	t.Helper()
	if value == nil || *value != want {
		t.Fatalf("optional value = %#v, want %q", value, want)
	}
}

func cloneStringMap(value map[string]string) map[string]string {
	copy := make(map[string]string, len(value))
	for key, item := range value {
		copy[key] = item
	}
	return copy
}
