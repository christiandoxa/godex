package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const kiroBuilderStartURL = "https://view.awsapps.com/start"

type Source struct {
	inspector  *Inspector
	getenv     func(string) string
	homeDir    func() (string, error)
	lookupPath func(string) (string, error)
	run        metadataRunner
}

type sourceSnapshot struct {
	AuthKey     string
	AuthJSON    string
	ProfileARN  string
	ProfileName string
	UserID      string
	StartURL    string
	Region      string
}

func NewSource() *Source {
	return &Source{
		inspector: NewInspector(), getenv: os.Getenv, homeDir: os.UserHomeDir,
		lookupPath: exec.LookPath, run: runMetadataCommand,
	}
}

func (source *Source) InspectAuthSecret(ctx context.Context, text string) (profilemodel.BuiltinCredential, error) {
	return source.inspector.InspectAuthSecret(ctx, text)
}

func (source *Source) ValidateModelCatalog(ctx context.Context, text string) error {
	return source.inspector.ValidateModelCatalog(ctx, text)
}

func (source *Source) Load(ctx context.Context) (profilemodel.BuiltinCredential, error) {
	databasePath, err := source.discoverDatabasePath()
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	snapshot, err := readSourceDatabase(ctx, databasePath)
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	whoami, _ := source.readWhoami(ctx, databasePath)
	credential, err := source.credentialFromSnapshot(ctx, snapshot, whoami)
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	catalog, catalogErr := source.readModelCatalog(ctx, databasePath, snapshot.Region)
	if catalogErr != nil {
		credential.Warning = "Kiro model catalog refresh failed; re-import this profile to retry."
		return credential, nil
	}
	credential.SecretFiles = append(credential.SecretFiles, profilemodel.ExportedSecretFile{
		Path: ModelCatalogFile, Text: catalog,
	})
	return credential, nil
}

func (source *Source) credentialFromSnapshot(ctx context.Context, snapshot sourceSnapshot, whoami map[string]any) (profilemodel.BuiltinCredential, error) {
	var token map[string]any
	if err := json.Unmarshal([]byte(snapshot.AuthJSON), &token); err != nil {
		return profilemodel.BuiltinCredential{}, fmt.Errorf("failed to parse Kiro auth JSON for key %q", snapshot.AuthKey)
	}
	startURL := firstString(token, "start_url", "startUrl")
	if snapshot.StartURL != "" {
		startURL = snapshot.StartURL
	}
	region := firstString(token, "region", "aws_region", "awsRegion")
	if snapshot.Region != "" {
		region = snapshot.Region
	}
	email := firstString(token, "email", "user_email", "userId", "user_id", "username")
	if email == "" {
		email = kiroEmail(whoami)
	}
	if email == "" {
		email = strings.TrimSpace(snapshot.UserID)
	}
	secret := authSecret{
		AuthKey: snapshot.AuthKey, AuthKind: kiroAuthKind(snapshot.AuthKey, token, startURL),
		AuthJSON: snapshot.AuthJSON, Email: optional(email), ProfileARN: optional(snapshot.ProfileARN),
		ProfileName: optional(snapshot.ProfileName), StartURL: optional(startURL), Region: optional(region),
	}
	content, err := json.MarshalIndent(secret, "", "  ")
	if err != nil {
		return profilemodel.BuiltinCredential{}, errors.New("failed to serialize Kiro auth snapshot")
	}
	return source.inspector.InspectAuthSecret(ctx, string(content))
}

func kiroAuthKind(authKey string, token map[string]any, startURL string) string {
	switch strings.TrimSpace(authKey) {
	case "kirocli:social:token":
		return "social"
	case "kirocli:external-idp:token":
		return "external-idp"
	}
	if candidate := firstString(token, "start_url", "startUrl"); candidate != "" {
		startURL = candidate
	}
	if value := strings.TrimSpace(startURL); value != "" && value != kiroBuilderStartURL {
		return "identity-center"
	}
	return "builder-id"
}

func kiroEmail(value map[string]any) string {
	candidate := firstString(value, "email", "user_email", "userId", "user_id")
	if candidate != "" {
		return candidate
	}
	username := firstString(value, "username")
	if strings.Contains(username, "@") {
		return username
	}
	return ""
}

func (source *Source) dataLocalDir() string {
	if configured := strings.TrimSpace(source.getenv("XDG_DATA_HOME")); configured != "" {
		return configured
	}
	if runtime.GOOS == "windows" {
		if configured := strings.TrimSpace(source.getenv("LOCALAPPDATA")); configured != "" {
			return configured
		}
	}
	home, _ := source.homeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support")
	case "windows":
		return filepath.Join(home, "AppData", "Local")
	default:
		return filepath.Join(home, ".local", "share")
	}
}
