package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const (
	CredentialsFile     = ".credentials.json"
	credentialsMaxBytes = 64 << 10
)

type Source struct {
	homeDir    func() (string, error)
	getenv     func(string) string
	oauthLogin oauthLoginRunner
}

type credentialsFile struct {
	ClaudeAIOAuth *credentialsToken `json:"claudeAiOauth"`
	AccessToken   string            `json:"accessToken"`
	ExpiresAt     *int64            `json:"expiresAt"`
	Subscription  string            `json:"subscriptionType"`
	Email         string            `json:"email"`
}

type credentialsToken struct {
	AccessToken  string `json:"accessToken"`
	ExpiresAt    *int64 `json:"expiresAt"`
	Subscription string `json:"subscriptionType"`
	Email        string `json:"email"`
}

func NewSource() *Source {
	return &Source{homeDir: os.UserHomeDir, getenv: os.Getenv}
}

func (source *Source) Load(ctx context.Context) (profilemodel.BuiltinCredential, error) {
	if err := ctx.Err(); err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	configDir, err := source.configDir()
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	text, err := readExternalCredential(configDir)
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	return source.InspectCredential(ctx, text)
}

func (source *Source) InspectCredential(ctx context.Context, text string) (profilemodel.BuiltinCredential, error) {
	if err := ctx.Err(); err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	if len(text) == 0 || len(text) > credentialsMaxBytes {
		return profilemodel.BuiltinCredential{}, errors.New("Claude credentials are empty or exceed the safe size limit")
	}
	account, authMethod, err := parseCredential(text)
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	provider := profilemodel.ProviderSnapshot{Kind: "anthropic"}
	if account != "" {
		copy := account
		provider.Account = &copy
	}
	if authMethod != "" {
		copy := authMethod
		provider.AuthMethod = &copy
	}
	return profilemodel.BuiltinCredential{
		Provider:    provider,
		Email:       account,
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: CredentialsFile, Text: text}},
	}, nil
}

func (source *Source) configDir() (string, error) {
	if configured := strings.TrimSpace(source.getenv("CLAUDE_CONFIG_DIR")); configured != "" {
		return validateConfigDir(configured)
	}
	home, err := source.homeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("failed to determine Claude config directory")
	}
	return validateConfigDir(filepath.Join(home, ".claude"))
}

func validateConfigDir(path string) (string, error) {
	clean := filepath.Clean(path)
	if strings.TrimSpace(path) == "" {
		return "", errors.New("Claude config directory is empty")
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return "", errors.New("Claude config directory path is unsafe")
		}
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return "", fmt.Errorf("failed to inspect Claude config directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("Claude config directory must be a real directory")
	}
	return clean, nil
}

func readExternalCredential(configDir string) (string, error) {
	path := filepath.Join(configDir, CredentialsFile)
	info, err := os.Lstat(path)
	if err != nil {
		return "", errors.New("Claude credentials file is missing")
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > credentialsMaxBytes {
		return "", errors.New("Claude credentials must be a bounded regular secret file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("failed to read Claude credentials")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, credentialsMaxBytes+1))
	if err != nil || len(content) > credentialsMaxBytes {
		return "", errors.New("failed to read Claude credentials")
	}
	return string(content), nil
}

func parseCredential(text string) (account, authMethod string, err error) {
	var file credentialsFile
	if err := json.Unmarshal([]byte(text), &file); err != nil {
		return "", "", errors.New("invalid Claude credentials JSON")
	}
	if file.ClaudeAIOAuth != nil {
		token := strings.TrimSpace(file.ClaudeAIOAuth.AccessToken)
		if token == "" {
			return "", "", errors.New("Claude credentials did not include an access token")
		}
		return strings.TrimSpace(file.ClaudeAIOAuth.Email), authMethodLabel(file.ClaudeAIOAuth.Subscription), nil
	}
	if strings.TrimSpace(file.AccessToken) == "" {
		return "", "", errors.New("Claude credentials did not include an access token")
	}
	return strings.TrimSpace(file.Email), authMethodLabel(file.Subscription), nil
}

func authMethodLabel(subscription string) string {
	value := strings.TrimSpace(subscription)
	if value == "" {
		return "claude-ai-oauth"
	}
	return "claude-ai-oauth:" + value
}
