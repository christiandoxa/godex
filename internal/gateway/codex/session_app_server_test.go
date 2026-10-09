//go:build !windows

package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	sessionCompanionHelperEnv  = "GODEX_TEST_SESSION_COMPANION_HELPER"
	sessionCompanionMarkerEnv  = "GODEX_TEST_SESSION_COMPANION_MARKER"
	sessionCompanionHelperName = "TestCodexSessionCompanionHelper"
	sessionCompanionMarkerMode = 0o600
)

func TestCodexSessionCompanionHelper(t *testing.T) {
	if os.Getenv(sessionCompanionHelperEnv) != "1" {
		return
	}
	arguments := os.Args
	for index, argument := range arguments {
		if argument == "--" {
			arguments = arguments[index+1:]
			break
		}
	}
	marker := os.Getenv(sessionCompanionMarkerEnv)
	if len(arguments) >= 1 && arguments[0] == "app-server" {
		listen := ""
		for index := 1; index+1 < len(arguments); index++ {
			if arguments[index] == "--listen" {
				listen = strings.TrimPrefix(arguments[index+1], "unix://")
				break
			}
		}
		if listen == "" {
			os.Exit(2)
		}
		listener, err := net.Listen("unix", listen)
		if err != nil {
			os.Exit(3)
		}
		defer listener.Close()
		appendSessionCompanionMarker(marker, "companion")
		stopped := make(chan os.Signal, 1)
		signal.Notify(stopped, syscall.SIGTERM, os.Interrupt)
		defer signal.Stop(stopped)
		<-stopped
		return
	}
	if hasSessionCompanionArgument(arguments, "--remote") {
		appendSessionCompanionMarker(marker, "child")
		if os.Getenv("GODEX_TEST_SESSION_COMPANION_HOLD_CHILD") == "1" {
			stopped := make(chan os.Signal, 1)
			signal.Notify(stopped, syscall.SIGTERM, os.Interrupt)
			defer signal.Stop(stopped)
			<-stopped
		}
		return
	}
	appendSessionCompanionMarker(marker, "direct")
}

func TestSessionAppServerCompanionArgumentsPreserveManagedConfig(t *testing.T) {
	got := sessionAppServerCompanionArguments([]string{
		"-c", `model_provider="godex-openai"`,
		"--model", "gpt-6.1-sol",
		"--strict-config",
		"--", "literal", "--model=ignored",
	}, "/tmp/home/.s")
	want := []string{
		"app-server", "--listen", "unix:///tmp/home/.s",
		"-c", `model_provider="godex-openai"`,
		"-c", `model="gpt-6.1-sol"`,
		"--strict-config",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("companion arguments = %#v, want %#v", got, want)
	}
}

func TestSessionAppServerCompanionArgumentsEncodeControlCharactersAsTOML(t *testing.T) {
	model := "synthetic\a-model"
	arguments := sessionAppServerCompanionArguments([]string{"--model", model}, "/tmp/home/.s")
	var modelConfig string
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == "-c" && strings.HasPrefix(arguments[index+1], "model=") {
			modelConfig = arguments[index+1]
			break
		}
	}
	if modelConfig == "" {
		t.Fatal("companion arguments did not include model config")
	}
	var decoded struct {
		Model string `toml:"model"`
	}
	if err := toml.Unmarshal([]byte(modelConfig+"\n"), &decoded); err != nil {
		t.Fatalf("companion model config is invalid TOML: %v (%q)", err, modelConfig)
	}
	if decoded.Model != model {
		t.Fatalf("decoded model = %q, want %q", decoded.Model, model)
	}
}

func TestSessionAppServerEligibilityDoesNotInspectOptionValuesAsCommands(t *testing.T) {
	if !sessionAppServerEligible([]string{"-c", "note=--remote", "--model", "synthetic"}) {
		t.Fatal("config value was mistaken for a remote launch")
	}
	if sessionAppServerEligible([]string{"--remote", "unix:///tmp/codex.sock"}) {
		t.Fatal("remote launch unexpectedly received a private companion")
	}
}

func appendSessionCompanionMarker(path, value string) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, sessionCompanionMarkerMode)
	if err != nil {
		os.Exit(4)
	}
	if _, err := fmt.Fprintln(file, value); err != nil {
		_ = file.Close()
		os.Exit(5)
	}
	if err := file.Close(); err != nil {
		os.Exit(6)
	}
}

func hasSessionCompanionArgument(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted || strings.HasPrefix(argument, wanted+"=") {
			return true
		}
	}
	return false
}

func TestRunThroughProxyWithSessionServerOwnsCompanionLifecycle(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	wrapper := sessionCompanionWrapper(t)
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(wrapper, Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	if err := process.RunThroughProxyWithSessionServer(t.Context(), home, "http://127.0.0.1:1234", []string{"--model", "synthetic"}, ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(mustSessionCompanionFile(t, marker))); got != "companion\nchild" {
		t.Fatalf("companion lifecycle = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(home, ".s")); !os.IsNotExist(err) {
		t.Fatalf("private app-server socket survived: %v", err)
	}
}

func TestRunThroughProxyWithLongSessionHomeReapsPrivateCompanion(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	wrapper := sessionCompanionWrapper(t)
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	home := filepath.Join(root, strings.Repeat("x", 100), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(wrapper, Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	if err := process.RunThroughProxyWithSessionServer(t.Context(), home, "http://127.0.0.1:1234", []string{"--model", "synthetic"}, ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(mustSessionCompanionFile(t, marker))); got != "companion\nchild" {
		t.Fatalf("long-home companion lifecycle = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(home, ".s")); !os.IsNotExist(err) {
		t.Fatalf("long-home socket survived: %v", err)
	}
}

func TestRunThroughProxyWithSessionServerLeavesNativeResumeUnchanged(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	wrapper := sessionCompanionWrapper(t)
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(wrapper, Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	if err := process.RunThroughProxyWithSessionServer(t.Context(), home, "http://127.0.0.1:1234", []string{"resume", "session-id"}, ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(mustSessionCompanionFile(t, marker))); got != "direct" {
		t.Fatalf("resume launch unexpectedly used companion: %q", got)
	}
}

func TestRunThroughProxyWithSessionServerCancellationReapsCompanion(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	wrapper := sessionCompanionWrapper(t)
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	t.Setenv("GODEX_TEST_SESSION_COMPANION_HOLD_CHILD", "1")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	process := NewCodexProcess(wrapper, Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	started := time.Now()
	err := process.RunThroughProxyWithSessionServer(ctx, home, "http://127.0.0.1:1234", []string{"--model", "synthetic"}, "")
	if !errorsIsContext(err) {
		t.Fatalf("cancellation error = %v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatalf("companion cancellation took too long: %v", time.Since(started))
	}
	if _, err := os.Lstat(filepath.Join(home, ".s")); !os.IsNotExist(err) {
		t.Fatalf("private app-server socket survived cancellation: %v", err)
	}
}

func sessionCompanionWrapper(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex-wrapper")
	content := "#!/bin/sh\nexec \"$GODEX_TEST_SESSION_COMPANION_BINARY\" -test.run=^" + sessionCompanionHelperName + "$ -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	binary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GODEX_TEST_SESSION_COMPANION_BINARY", binary)
	return path
}

func mustSessionCompanionFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func errorsIsContext(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}
