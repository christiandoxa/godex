package claude

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProdex04356ClaudeOAuthLoginUsesIsolatedConfigAndSanitizedEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ANTHROPIC_API_KEY", "<redacted>")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "<redacted>")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "<redacted>")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "wrong"))

	source := NewSource()
	var gotDir string
	var gotArgs, gotEnv []string
	source.oauthLogin = func(_ context.Context, configDir string, args, environment []string) error {
		gotDir = configDir
		gotArgs = append([]string(nil), args...)
		gotEnv = append([]string(nil), environment...)
		content := []byte("{\"claudeAiOauth\":{\"accessToken\":\"fixture-access\",\"subscriptionType\":\"pro\",\"email\":\"person@example.test\"}}")
		return os.WriteFile(filepath.Join(configDir, CredentialsFile), content, 0o600)
	}

	credential, err := source.LoginOAuth(t.Context(), home, "")
	if err != nil {
		t.Fatal(err)
	}
	if gotDir != home || !slices.Equal(gotArgs, []string{"auth", "login", "--claudeai"}) {
		t.Fatalf("Claude OAuth dir/args = %q / %#v", gotDir, gotArgs)
	}
	joined := "\n" + strings.Join(gotEnv, "\n") + "\n"
	for _, forbidden := range []string{
		"\nANTHROPIC_API_KEY=", "\nANTHROPIC_AUTH_TOKEN=",
		"\nCLAUDE_CODE_OAUTH_TOKEN=",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("Claude OAuth env retained %q", forbidden)
		}
	}
	if !strings.Contains(joined, "\nCLAUDE_CONFIG_DIR="+home+"\n") {
		t.Fatalf("Claude OAuth env missing isolated config: %#v", gotEnv)
	}
	if credential.Provider.Kind != "anthropic" || credential.Email != "person@example.test" ||
		credential.Provider.AuthMethod == nil || *credential.Provider.AuthMethod != "claude-ai-oauth:pro" {
		t.Fatalf("Claude OAuth credential = %#v", credential)
	}
}

func TestProdex04356ClaudeOAuthLoginForwardsOptionalEmail(t *testing.T) {
	home := t.TempDir()
	source := NewSource()
	source.oauthLogin = func(_ context.Context, configDir string, args, _ []string) error {
		if !slices.Equal(args, []string{"auth", "login", "--claudeai", "--email", "person@example.test"}) {
			t.Fatalf("Claude OAuth email args = %#v", args)
		}
		content := []byte("{\"accessToken\":\"fixture-access\",\"email\":\"person@example.test\"}")
		return os.WriteFile(filepath.Join(configDir, CredentialsFile), content, 0o600)
	}
	if _, err := source.LoginOAuth(t.Context(), home, " person@example.test "); err != nil {
		t.Fatal(err)
	}
}
