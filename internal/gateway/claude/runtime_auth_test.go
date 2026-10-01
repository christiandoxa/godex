package claude

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeOAuthRefreshesExpiredManagedCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	home := t.TempDir()
	writeClaudeCredential(t, home, `{"claudeAiOauth":{"accessToken":"expired-token","expiresAt":1}}`)
	binary := filepath.Join(t.TempDir(), "claude-fixture")
	script := `#!/bin/sh
set -eu
[ "$1" = auth ]
[ "$2" = status ]
[ "$3" = --json ]
[ -n "${CLAUDE_CONFIG_DIR:-}" ]
[ -z "${ANTHROPIC_API_KEY:-}" ]
[ -z "${ANTHROPIC_AUTH_TOKEN:-}" ]
[ -z "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]
cat > "$CLAUDE_CONFIG_DIR/.credentials.json" <<'JSON'
{"claudeAiOauth":{"accessToken":"refreshed-token","expiresAt":4102444800000}}
JSON
printf '{"loggedIn":true}\n'
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_BIN", binary)
	t.Setenv("ANTHROPIC_API_KEY", "must-be-removed")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "must-be-removed")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "must-be-removed")
	auth, err := NewSource().RuntimeOAuth(context.Background(), home)
	if err != nil || auth.accessToken != "refreshed-token" {
		t.Fatalf("auth = %#v, err = %v", auth, err)
	}
}

func TestRuntimeOAuthRejectsStillExpiredCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	home := t.TempDir()
	writeClaudeCredential(t, home, `{"accessToken":"expired-token","expiresAt":1}`)
	binary := filepath.Join(t.TempDir(), "claude-fixture")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_BIN", binary)
	if _, err := NewSource().RuntimeOAuth(context.Background(), home); err == nil || !strings.Contains(err.Error(), "remained expired") {
		t.Fatalf("error = %v", err)
	}
}

func writeClaudeCredential(t *testing.T, home, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, CredentialsFile), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
