package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	oauthExpirySkew   = 60 * time.Second
	authProbeTimeout  = 15 * time.Second
	authProbeMaxBytes = 1 << 20
)

type RuntimeOAuth struct {
	accessToken string
}

type oauthSecret struct {
	accessToken string
	expiresAt   *int64
}

type boundedCommandBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (buffer *boundedCommandBuffer) Write(content []byte) (int, error) {
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		write := min(len(content), remaining)
		_, _ = buffer.buffer.Write(content[:write])
	}
	return len(content), nil
}

func (source *Source) RuntimeOAuth(ctx context.Context, home string) (RuntimeOAuth, error) {
	secret, err := readManagedOAuthSecret(home)
	if err != nil {
		return RuntimeOAuth{}, err
	}
	if oauthExpired(secret, time.Now()) {
		if err := source.refreshManagedOAuth(ctx, home); err != nil {
			return RuntimeOAuth{}, err
		}
		secret, err = readManagedOAuthSecret(home)
		if err != nil {
			return RuntimeOAuth{}, err
		}
		if oauthExpired(secret, time.Now()) {
			return RuntimeOAuth{}, errors.New("Claude OAuth credential remained expired after refresh")
		}
	}
	return RuntimeOAuth{accessToken: secret.accessToken}, nil
}

func readManagedOAuthSecret(home string) (oauthSecret, error) {
	if strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return oauthSecret{}, errors.New("Claude runtime profile home must be absolute")
	}
	text, err := readExternalCredential(filepath.Clean(home))
	if err != nil {
		return oauthSecret{}, err
	}
	return parseOAuthSecret(text)
}

func parseOAuthSecret(text string) (oauthSecret, error) {
	var file credentialsFile
	if err := json.Unmarshal([]byte(text), &file); err != nil {
		return oauthSecret{}, errors.New("invalid Claude credentials JSON")
	}
	if file.ClaudeAIOAuth != nil {
		token := strings.TrimSpace(file.ClaudeAIOAuth.AccessToken)
		if token == "" {
			return oauthSecret{}, errors.New("Claude credentials did not include an access token")
		}
		return oauthSecret{accessToken: token, expiresAt: file.ClaudeAIOAuth.ExpiresAt}, nil
	}
	token := strings.TrimSpace(file.AccessToken)
	if token == "" {
		return oauthSecret{}, errors.New("Claude credentials did not include an access token")
	}
	return oauthSecret{accessToken: token, expiresAt: file.ExpiresAt}, nil
}

func oauthExpired(secret oauthSecret, now time.Time) bool {
	if secret.expiresAt == nil {
		return false
	}
	deadline := now.Add(oauthExpirySkew).UnixMilli()
	return *secret.expiresAt <= deadline
}

func (source *Source) refreshManagedOAuth(ctx context.Context, home string) error {
	binary := strings.TrimSpace(source.getenv("CLAUDE_BIN"))
	if binary == "" {
		binary = "claude"
	}
	runCtx, cancel := context.WithTimeout(ctx, authProbeTimeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, "auth", "status", "--json")
	command.Env = claudeRuntimeEnvironment(home)
	stdout := &boundedCommandBuffer{limit: authProbeMaxBytes}
	stderr := &boundedCommandBuffer{limit: authProbeMaxBytes}
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if runCtx.Err() != nil {
			return errors.New("Claude auth refresh timed out")
		}
		return errors.New("Claude auth status failed while refreshing OAuth credential")
	}
	return nil
}

func claudeRuntimeEnvironment(home string) []string {
	blocked := map[string]bool{
		"CLAUDE_CONFIG_DIR": true, "ANTHROPIC_API_KEY": true,
		"ANTHROPIC_AUTH_TOKEN": true, "CLAUDE_CODE_OAUTH_TOKEN": true,
	}
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found && blocked[key] {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "CLAUDE_CONFIG_DIR="+home)
}
