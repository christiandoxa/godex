package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/christiandoxa/godex/internal/config"
)

func TestNativeAntigravityEntrypointRecognizesProdexSyntax(t *testing.T) {
	for _, input := range [][]string{
		{"s", "gemini", "--cli", "agy"},
		{"super", "gemini", "--cli", "agy"},
		{"s", "--provider", "gemini", "--cli", "agy"},
		{"s", "--no-presidio", "--no-sub-agent", "gemini", "--cli", "agy"},
	} {
		arguments, ok := nativeAntigravityArguments(input)
		if !ok {
			t.Fatalf("native Antigravity did not recognize Prodex syntax %#v", input)
		}
		want := []string{"--provider", "gemini", "--cli", "agy"}
		if input[1] == "--provider" {
			want = input[1:]
		} else if input[1] == "--no-presidio" {
			want = []string{"--no-presidio", "--no-sub-agent", "--provider", "gemini", "--cli", "agy"}
		}
		if !reflect.DeepEqual(arguments, want) {
			t.Fatalf("normalized Prodex syntax = %#v, want %#v", arguments, want)
		}
	}
	if _, ok := nativeAntigravityArguments([]string{"s", "gemini"}); ok {
		t.Fatal("Super syntax without native Antigravity was intercepted")
	}
	for _, command := range []string{"doctor", "login", "quota", "profile", "help"} {
		if _, ok := nativeAntigravityArguments([]string{command, "--provider", "gemini", "--cli", "agy"}); ok {
			t.Fatalf("Antigravity intercepted explicit %s command", command)
		}
	}
}

func TestNativeAntigravityEntrypointDryRunUsesMinimalStartup(t *testing.T) {
	t.Setenv(config.HomeEnv, t.TempDir())
	t.Setenv(config.CodexHomeEnv, string(os.PathSeparator))
	t.Setenv(config.UpstreamEnv, "not-a-valid-upstream")
	t.Setenv(config.ProdexHomeEnv, t.TempDir())
	sharedHome := filepath.Join(t.TempDir(), "shared-codex")
	t.Setenv(config.ProdexSharedCodexHomeEnv, sharedHome)
	t.Setenv(config.AgyBinEnv, filepath.Join(t.TempDir(), "missing-agy"))

	original := os.Args
	os.Args = []string{
		"godex", "s", "gemini", "--no-presidio", "--no-sub-agent", "--cli", "agy",
		"--model", "gpt-6-luna", "--dry-run", "-c", "model_provider=\"openai\"", "exec",
	}
	t.Cleanup(func() { os.Args = original })
	if got := run(); got != 0 {
		t.Fatalf("minimal Antigravity dry-run exit code = %d", got)
	}
	info, err := os.Stat(sharedHome)
	if err != nil || !info.IsDir() {
		t.Fatalf("dry-run shared CODEX_HOME info = %#v, err=%v", info, err)
	}
}

func TestNativeAntigravityExitDiagnosticPreservesChildCode(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestExitCodeChild$")
	command.Env = append(os.Environ(), "GODEX_EXIT_CODE_HELPER=1")
	err := command.Run()
	var stderr bytes.Buffer
	if got := antigravityExitCode(context.Background(), err, &stderr); got != 23 {
		t.Fatalf("Antigravity exit code = %d, want 23", got)
	}
	if got, want := stderr.String(), "Error: Antigravity CLI exited unsuccessfully\n"; got != want {
		t.Fatalf("Antigravity child diagnostic = %q, want %q", got, want)
	}
}
