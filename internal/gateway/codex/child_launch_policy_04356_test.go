package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProdex04356CodexChildEnvironmentHardeningAndLoopbackBypass(t *testing.T) {
	t.Setenv("CODEX_SANDBOX", "1")
	t.Setenv("CODEX_SANDBOX_NETWORK_DISABLED", "1")
	t.Setenv("CODEX_SANDBOX_CUSTOM", "1")
	t.Setenv("LD_PRELOAD", "/synthetic/preload.so")
	t.Setenv("LD_AUDIT", "/synthetic/audit.so")
	t.Setenv("DYLD_CUSTOM_INJECTION", "/synthetic/dyld")
	t.Setenv("NO_PROXY", "corp.example")
	t.Setenv("no_proxy", "other.example")
	if err := os.Unsetenv("GODEX_ALLOW_UNSAFE_CHILD_ENV"); err != nil {
		t.Fatal(err)
	}

	environment := environmentWith("CODEX_HOME", t.TempDir())
	joined := strings.Join(environment, "\n")
	for _, forbidden := range []string{
		"CODEX_SANDBOX=", "CODEX_SANDBOX_NETWORK_DISABLED=", "CODEX_SANDBOX_CUSTOM=",
		"LD_PRELOAD=", "LD_AUDIT=", "DYLD_CUSTOM_INJECTION=",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unsafe child env %q survived:\n%s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "CODEX_TUI_DISABLE_KEYBOARD_ENHANCEMENT=1") {
		t.Fatalf("keyboard enhancement guard missing:\n%s", joined)
	}
	for _, key := range []string{"NO_PROXY=", "no_proxy="} {
		line := childEnvironmentLine(environment, key)
		for _, want := range []string{"127.0.0.1", "localhost", "::1"} {
			if !strings.Contains(line, want) {
				t.Fatalf("%s missing %q: %q", key, want, line)
			}
		}
	}
}

func TestProdex04356UnsafeChildEnvEscapeHatchDoesNotRestoreSandboxMarkers(t *testing.T) {
	t.Setenv("GODEX_ALLOW_UNSAFE_CHILD_ENV", "1")
	t.Setenv("LD_PRELOAD", "/synthetic/preload.so")
	t.Setenv("CODEX_SANDBOX", "1")

	joined := strings.Join(environmentWith("CODEX_HOME", t.TempDir()), "\n")
	if !strings.Contains(joined, "LD_PRELOAD=/synthetic/preload.so") {
		t.Fatalf("unsafe-child opt-in did not preserve loader env:\n%s", joined)
	}
	if strings.Contains(joined, "CODEX_SANDBOX=1") {
		t.Fatalf("sandbox marker survived unsafe-child opt-in:\n%s", joined)
	}
}

func TestProdex04356CodexTUILaunchAddsPasteBurstOverrideOnlyWhenApplicable(t *testing.T) {
	tui, err := PreviewRuntimeProxyArguments("", []string{"resume", "thread-id"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsArgumentPair(tui, "-c", "disable_paste_burst=true") {
		t.Fatalf("TUI preview missing paste-burst guard: %#v", tui)
	}

	execArgs, err := PreviewRuntimeProxyArguments("", []string{"exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if containsArgumentPair(execArgs, "-c", "disable_paste_burst=true") {
		t.Fatalf("exec preview unexpectedly gained paste-burst guard: %#v", execArgs)
	}

	serverArgs, err := PreviewRuntimeProxyArguments("", []string{"app-server"})
	if err != nil {
		t.Fatal(err)
	}
	if containsArgumentPair(serverArgs, "-c", "disable_paste_burst=true") {
		t.Fatalf("command-server preview unexpectedly gained paste-burst guard: %#v", serverArgs)
	}

	explicit, err := PreviewRuntimeProxyArguments("", []string{
		"-c", "disable_paste_burst=false", "resume", "thread-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for index := 0; index+1 < len(explicit); index++ {
		if explicit[index] == "-c" && strings.HasPrefix(explicit[index+1], "disable_paste_burst=") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("explicit paste-burst override was duplicated: %#v", explicit)
	}
}

func TestProdex04356KeyboardEnhancementPopSequenceMatchesCrossterm(t *testing.T) {
	var output bytes.Buffer
	if _, err := output.WriteString(terminalKeyboardEnhancementPop); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "\x1b[<1u"; got != want {
		t.Fatalf("keyboard enhancement pop = %q, want %q", got, want)
	}
}

func TestProdex04356DirectCodexRunAppliesTUILaunchPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	record := filepath.Join(t.TempDir(), "args.txt")
	script := filepath.Join(t.TempDir(), "codex-helper.sh")
	content := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$GODEX_CHILD_POLICY_RECORD\"\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GODEX_CHILD_POLICY_RECORD", record)
	process := NewCodexProcess(script, Terminal{})
	if err := process.RunRuntime(context.Background(), t.TempDir(), []string{"resume", "thread-id"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "disable_paste_burst=true") || !strings.Contains(text, "resume\nthread-id\n") {
		t.Fatalf("direct child argv missing TUI policy: %q", text)
	}
}

func childEnvironmentLine(environment []string, prefix string) string {
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return entry
		}
	}
	return ""
}

func containsArgumentPair(arguments []string, key, value string) bool {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == key && arguments[index+1] == value {
			return true
		}
	}
	return false
}
