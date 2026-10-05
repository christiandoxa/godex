package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCheckProxySupportAcceptsAndRejectsCapability(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX executable")
	}
	tests := []struct {
		name     string
		exit     string
		wantErr  bool
		relative bool
	}{
		{name: "supported", exit: "0"},
		{name: "relative executable", exit: "0", relative: true},
		{name: "unsupported", exit: "1", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertProxySupportCase(t, test.exit, test.wantErr, test.relative)
		})
	}
}

func assertProxySupportCase(t *testing.T, exit string, wantErr, relative bool) {
	t.Helper()
	script := writeProxyHelper(t, proxySupportScript(exit))
	if relative {
		script = relativeProxyHelperPath(t, script)
	}
	err := NewCodexProcess(script, Terminal{}).CheckProxySupport(context.Background())
	if (err != nil) != wantErr {
		t.Fatalf("capability error = %v", err)
	}
	if wantErr && !strings.Contains(err.Error(), "upgrade Codex") {
		t.Fatalf("capability error = %v", err)
	}
}

func proxySupportScript(exit string) string {
	return `if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then
  printf 'codex-cli 0.159.2\n'
  exit 0
fi
[ "$1" = "--strict-config" ] || exit 2
shift
while [ "$1" = "-c" ]; do shift 2; done
[ "$*" = "exec-server --listen stdio" ] || exit 2
[ ! -f "$CODEX_HOME/auth.json" ] || exit 2
[ "$(pwd -P)" = "$(cd "$CODEX_HOME" && pwd -P)" ] || exit 2
exit ` + exit
}

func relativeProxyHelperPath(t *testing.T, script string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, script)
	if err != nil {
		t.Fatal(err)
	}
	return relative
}

func TestRunThroughProxyPreservesArgumentsAndHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX executable")
	}
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("GODEX_PROXY_RECORD", record)
	script := writeProxyHelper(t, `printf '%s' "$CODEX_HOME" > "$GODEX_PROXY_RECORD.home"
printf '%s\n' "$@" > "$GODEX_PROXY_RECORD.args"`)
	home := t.TempDir()
	if err := NewCodexProcess(script, Terminal{}).RunThroughProxy(context.Background(), home, "http://127.0.0.1:1234", []string{"--model", "synthetic"}); err != nil {
		t.Fatal(err)
	}
	childHome, err := os.ReadFile(record + ".home")
	if err != nil || string(childHome) != home {
		t.Fatalf("child home = %q, err = %v", childHome, err)
	}
	arguments, err := os.ReadFile(record + ".args")
	if err != nil || !strings.Contains(string(arguments), `model_providers.godex-openai.base_url="http://127.0.0.1:1234/backend-api/prodex"`) || !strings.Contains(string(arguments), "--model\nsynthetic") {
		t.Fatalf("child arguments = %q, err = %v", arguments, err)
	}
	if err := NewCodexProcess(script, Terminal{}).RunThroughProxy(context.Background(), home, "http://127.0.0.1:1234", []string{"-c", "openai_base_url=https://outside"}); err == nil {
		t.Fatal("proxy override unexpectedly accepted")
	}
	if err := NewCodexProcess(script, Terminal{}).RunThroughProxy(context.Background(), "/", "http://127.0.0.1:1234", nil); err == nil {
		t.Fatal("proxy with filesystem root unexpectedly accepted")
	}
}

func writeProxyHelper(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestManagedTransportCannotBeOverridden(t *testing.T) {
	for _, key := range []string{"model_provider", "model_providers.evil.base_url", "model_providers.godex-openai.supports_websockets", "cli_auth_credentials_store"} {
		if _, err := proxyArguments("http://127.0.0.1:1234", []string{"-c", key + "=true"}); err == nil {
			t.Fatalf("override accepted: %s", key)
		}
	}
	args, err := proxyArguments("http://127.0.0.1:1234", []string{"exec", "synthetic prompt"})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(args, " ")
	if strings.Contains(text, "chatgpt_base_url=") || !strings.Contains(text, "supports_websockets=false") {
		t.Fatalf("native bootstrap or HTTP transport not preserved")
	}
}

func TestQuotedAndEqualsConfigCannotBypassManagedRouting(t *testing.T) {
	for _, args := range [][]string{
		{"-c=model_provider=evil"}, {"--config=model_provider=evil"}, {"-cmodel_provider=evil"},
		{"--config", `"model_provider"="evil"`}, {"-c", `"model\u005fprovider"="evil"`},
		{"-c", "'cli_auth_credentials_store'='keyring'"}, {"-c", "model_providers={evil=true}"},
		{"-c", `profiles.work.model_provider="evil"`}, {"-c", `"model_providers".'godex-openai'.base_url="https://example.test"`},
		{"--oss"}, {"--oss=true"}, {"--local-provider", "ollama"}, {"--local-provider=lmstudio"},
		{"--remote", "ws://127.0.0.1:1"}, {"--remote=ws://127.0.0.1:1"},
		{"--remote-auth-token-env", "SYNTHETIC_REMOTE_TOKEN"}, {"--remote-auth-token-env=SYNTHETIC_REMOTE_TOKEN"},
		{"exec", "resume", "thread", "--config", "cli_auth_credentials_store=keyring"},
	} {
		if _, err := proxyArguments("http://127.0.0.1:1", args); err == nil {
			t.Fatalf("routing bypass accepted: %v", args)
		}
	}
	prompt := []string{"--", "-c", "model_provider=literal prompt", "--oss", "--local-provider=literal"}
	args, err := proxyArguments("http://127.0.0.1:1", append([]string{"exec"}, prompt...))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args[len(args)-len(prompt):], prompt) {
		t.Fatal("prompt changed")
	}
}

func TestExecConfigurationStaysInInnermostScope(t *testing.T) {
	for _, test := range []struct {
		arguments, want []string
	}{
		{[]string{"-c", "model=first", "exec", "--json", "prompt", "-c", "model=last"}, []string{"exec", "managed", "-c", "model=first", "--json", "prompt", "-c", "model=last"}},
		{[]string{"--config=model=first", "exec", "--json", "resume", "thread", "prompt"}, []string{"exec", "--json", "resume", "managed", "--config=model=first", "thread", "prompt"}},
		{[]string{"-c=model=first", "e", "fork", "thread", "prompt"}, []string{"e", "fork", "managed", "-c=model=first", "thread", "prompt"}},
		{[]string{"-cmodel=first", "exec", "review", "--uncommitted"}, []string{"exec", "review", "managed", "-cmodel=first", "--uncommitted"}},
		{[]string{"--model", "resume", "exec", "--add-dir", "fork", "resume", "thread"}, []string{"--model", "resume", "exec", "--add-dir", "fork", "resume", "managed", "thread"}},
		{[]string{"--thread-source", "review", "exec", "--", "resume"}, []string{"--thread-source", "review", "exec", "managed", "--", "resume"}},
		{[]string{"exec", "prompt", "resume"}, []string{"exec", "managed", "prompt", "resume"}},
		{[]string{"--", "exec", "resume"}, []string{"managed", "--", "exec", "resume"}},
		{[]string{"resume", "thread"}, []string{"managed", "resume", "thread"}},
		{[]string{"fork", "thread"}, []string{"managed", "fork", "thread"}},
		{[]string{"review", "--uncommitted"}, []string{"managed", "review", "--uncommitted"}},
	} {
		if got := scopeModelArguments(test.arguments, []string{"managed"}); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("scope %v: got %v, want %v", test.arguments, got, test.want)
		}
	}
}

// Opt-in parser smoke: an unknown strict-config field stops before any model work.
func TestInstalledCodexConfigSmoke(t *testing.T) {
	binary := os.Getenv("GODEX_TEST_CODEX_BIN")
	if binary == "" {
		t.Skip("set GODEX_TEST_CODEX_BIN for a local parser smoke")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	process := NewCodexProcess(binary, Terminal{})
	if err := process.CheckProxySupport(ctx); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{
		{"exec", "prompt"}, {"exec", "resume", "synthetic-thread", "prompt"},
		{"exec", "fork", "synthetic-thread", "prompt"}, {"exec", "review", "--uncommitted"},
		{"resume", "synthetic-thread"}, {"fork", "synthetic-thread"}, {"review", "--uncommitted"},
	} {
		var diagnostic bytes.Buffer
		process := NewCodexProcess(binary, Terminal{Stderr: &diagnostic})
		arguments := append([]string{"--strict-config", "-c", "godex_probe_unknown=true"}, command...)
		err := process.RunThroughProxy(ctx, t.TempDir(), "http://127.0.0.1:1", arguments)
		if err == nil || !strings.Contains(diagnostic.String(), "unknown configuration field `godex_probe_unknown`") {
			t.Fatalf("%v did not stop at strict config validation: %v, %s", command, err, diagnostic.String())
		}
	}
}

func TestProxyChildEnvironmentScrubsProviderAPISecretsForExternalProviders(t *testing.T) {
	secretEnvironment := map[string]string{
		"OPENAI_API_KEYS":         "openai-many",
		"OPENAI_API_KEY":          "openai-one",
		"ANTHROPIC_API_KEYS":      "anthropic-many",
		"ANTHROPIC_API_KEY":       "anthropic-one",
		"DEEPSEEK_API_KEYS":       "deepseek-many",
		"DEEPSEEK_API_KEY":        "deepseek-one",
		"GEMINI_API_KEYS":         "gemini-many",
		"GEMINI_API_KEY":          "gemini-one",
		"GOOGLE_API_KEYS":         "google-many",
		"GOOGLE_API_KEY":          "google-one",
		"GITHUB_COPILOT_API_KEYS": "copilot-many",
		"GITHUB_COPILOT_API_KEY":  "copilot-one",
	}
	for key, value := range secretEnvironment {
		t.Setenv(key, value)
	}
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "keep-auth-token")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "keep-oauth-token")
	t.Setenv("GODEX_UNRELATED_ENV", "keep-me")
	home := t.TempDir()

	external := strings.Join(proxyChildEnvironment(home, "anthropic", ""), "\n")
	for key := range secretEnvironment {
		if strings.Contains(external, key+"=") {
			t.Fatalf("external-provider secret environment %q leaked", key)
		}
	}
	for _, value := range []string{
		"CODEX_HOME=" + home,
		"GODEX_UNRELATED_ENV=keep-me",
		"ANTHROPIC_AUTH_TOKEN=keep-auth-token",
		"CLAUDE_CODE_OAUTH_TOKEN=keep-oauth-token",
	} {
		if !strings.Contains(external, value) {
			t.Fatalf("external-provider child environment lost safe value %q", value)
		}
	}

	openAI := strings.Join(proxyChildEnvironment(home, "", ""), "\n")
	if !strings.Contains(openAI, "ANTHROPIC_API_KEY=anthropic-one") {
		t.Fatal("managed OpenAI child environment was unexpectedly scrubbed")
	}
}

func TestRunThroughProxyUsesSharedSQLiteHomeForNativePicker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX executable and symlink setup")
	}
	root := t.TempDir()
	home := filepath.Join(root, "profile")
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, "record")
	t.Setenv("GODEX_PROXY_RECORD", record)
	script := writeProxyHelper(t, `printf '%s\n%s\n' "$CODEX_HOME" "$CODEX_SQLITE_HOME" > "$GODEX_PROXY_RECORD"`)
	process := NewCodexProcess(script, Terminal{})
	process.SetSharedCodexHome(shared)
	if err := process.PrepareSharedSessionHome(home, shared); err != nil {
		t.Fatal(err)
	}
	if err := process.RunThroughProxy(t.Context(), home, "http://127.0.0.1:1234", []string{"resume"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 || lines[0] != home || lines[1] != shared {
		t.Fatalf("native picker homes = %#v, want [%q %q]", lines, home, shared)
	}
}
