//go:build !windows

package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	sessionCompanionHelperEnv           = "GODEX_TEST_SESSION_COMPANION_HELPER"
	sessionCompanionMarkerEnv           = "GODEX_TEST_SESSION_COMPANION_MARKER"
	sessionCompanionArgsEnv             = "GODEX_TEST_SESSION_COMPANION_ARGS"
	sessionCompanionTreeEnv             = "GODEX_TEST_SESSION_COMPANION_TREE"
	sessionCompanionForegroundTreeEnv   = "GODEX_TEST_SESSION_COMPANION_FOREGROUND_TREE"
	sessionCompanionForegroundNativeEnv = "GODEX_TEST_SESSION_COMPANION_FOREGROUND_NATIVE"
	sessionCompanionStartupFailureEnv   = "GODEX_TEST_SESSION_COMPANION_STARTUP_FAILURE"
	sessionCompanionExitCodeEnv         = "GODEX_TEST_SESSION_COMPANION_EXIT_CODE"
	sessionCompanionHelperName          = "TestCodexSessionCompanionHelper"
	sessionCompanionMarkerMode          = 0o600
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
	if len(arguments) >= 1 && arguments[0] == "native" {
		runSessionCompanionNative(arguments, marker)
		return
	}
	if len(arguments) >= 1 && arguments[0] == "app-server" {
		if os.Getenv(sessionCompanionStartupFailureEnv) == "1" {
			os.Exit(18)
		}
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
		if argsPath := os.Getenv(sessionCompanionArgsEnv); argsPath != "" {
			if err := os.WriteFile(argsPath, []byte(strings.Join(arguments, "\x00")), sessionCompanionMarkerMode); err != nil {
				os.Exit(6)
			}
		}
		if os.Getenv(sessionCompanionTreeEnv) == "1" {
			runSessionCompanionWrapper(listen, marker)
			return
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
		if code := os.Getenv(sessionCompanionExitCodeEnv); code != "" {
			exitCode, err := strconv.Atoi(code)
			if err == nil {
				os.Exit(exitCode)
			}
		}
		if os.Getenv(sessionCompanionForegroundTreeEnv) == "1" {
			runSessionCompanionForegroundWrapper(marker)
			return
		}
		if os.Getenv(sessionCompanionTreeEnv) == "1" {
			waitForSessionCompanionFile(filepath.Join(filepath.Dir(marker), "companion-exited"))
			time.Sleep(100 * time.Millisecond)
		}
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

func runSessionCompanionForegroundWrapper(marker string) {
	root := filepath.Dir(marker)
	readySocket := filepath.Join(root, "foreground-ready.sock")
	child := exec.Command(os.Args[0], "-test.run=^"+sessionCompanionHelperName+"$", "--", "native", readySocket)
	child.Env = append(os.Environ(), sessionCompanionHelperEnv+"=1", sessionCompanionForegroundNativeEnv+"=1")
	if err := child.Start(); err != nil {
		os.Exit(19)
	}
	waitForSessionCompanionFile(filepath.Join(root, "native-ready"))
	select {}
}

func runSessionCompanionWrapper(listen, marker string) {
	child := exec.Command(os.Args[0], "-test.run=^"+sessionCompanionHelperName+"$", "--", "native", listen)
	child.Env = append(os.Environ(), sessionCompanionHelperEnv+"=1")
	if err := child.Start(); err != nil {
		os.Exit(7)
	}
	waitForSessionCompanionFile(filepath.Join(filepath.Dir(marker), "native-ready"))
	appendSessionCompanionMarker(marker, "companion")
	waitForSessionCompanionFile(filepath.Join(filepath.Dir(marker), "outer-connected"))
	// Let the launcher exit with its native child still alive.
	if err := os.WriteFile(filepath.Join(filepath.Dir(marker), "companion-exited"), []byte("done"), sessionCompanionMarkerMode); err != nil {
		os.Exit(9)
	}
}

func runSessionCompanionNative(arguments []string, marker string) {
	if len(arguments) < 2 {
		os.Exit(10)
	}
	// Keep the synthetic child alive to exercise group cleanup after reaping.
	clearSessionCompanionParentDeath()
	root := filepath.Dir(marker)
	livePath := filepath.Join(root, "native-live")
	if err := os.WriteFile(livePath, []byte("live"), sessionCompanionMarkerMode); err != nil {
		os.Exit(11)
	}
	if err := os.WriteFile(filepath.Join(root, "native-pid"), []byte(strconv.Itoa(os.Getpid())), sessionCompanionMarkerMode); err != nil {
		os.Exit(13)
	}
	if err := os.WriteFile(filepath.Join(root, "native-pgid"), []byte(strconv.Itoa(syscall.Getpgrp())), sessionCompanionMarkerMode); err != nil {
		os.Exit(14)
	}
	var ready net.Listener
	var err error
	if os.Getenv(sessionCompanionForegroundNativeEnv) != "1" {
		ready, err = net.Listen("unix", arguments[1])
		if err != nil {
			os.Exit(12)
		}
		defer ready.Close()
		go func() {
			connection, acceptErr := ready.Accept()
			if acceptErr == nil {
				_ = connection.Close()
				_ = os.WriteFile(filepath.Join(root, "outer-connected"), []byte("done"), sessionCompanionMarkerMode)
			}
		}()
	}
	if err := os.WriteFile(filepath.Join(root, "native-ready"), []byte("done"), sessionCompanionMarkerMode); err != nil {
		os.Exit(17)
	}
	appendSessionCompanionMarker(marker, "native")
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(stopped)
	<-stopped
}

func waitForSessionCompanionFile(path string) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(15)
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

func TestSessionAppServerCompanionArgumentsPreserveFullAccessPolicy(t *testing.T) {
	got := sessionAppServerCompanionArguments([]string{
		"--dangerously-bypass-approvals-and-sandbox", "--model", "synthetic",
	}, "/tmp/home/.s")
	want := []string{
		"app-server", "--listen", "unix:///tmp/home/.s",
		"-c", `model="synthetic"`,
		"-c", `approval_policy="never"`,
		"-c", `sandbox_mode="danger-full-access"`,
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("full-access companion arguments = %#v, want %#v", got, want)
	}
}

func TestSessionAppServerEligibilityDoesNotInspectOptionValuesAsCommands(t *testing.T) {
	if !sessionAppServerEligible([]string{"-c", "note=--remote", "--model", "synthetic"}) {
		t.Fatal("config value was mistaken for a remote launch")
	}
	if sessionAppServerEligible([]string{"--remote", "unix:///tmp/codex.sock"}) {
		t.Fatal("remote launch unexpectedly received a private companion")
	}
	if sessionAppServerEligible([]string{"app-server", "proxy"}) {
		t.Fatal("explicit app-server launch unexpectedly received a private companion")
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

func TestRunThroughProxyWithSessionServerReapsWrappedNativeDescendant(t *testing.T) {
	root := t.TempDir()
	defer stopSessionCompanionNative(root)
	marker := filepath.Join(root, "events")
	wrapper := sessionCompanionTreeWrapper(t)
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	t.Setenv(sessionCompanionTreeEnv, "1")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	unrelated := exec.Command("sleep", "30")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = unrelated.Process.Kill()
		_ = unrelated.Wait()
	}()
	foregroundGroup := syscall.Getpgrp()
	process := NewCodexProcess(wrapper, Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	if err := process.RunThroughProxyWithSessionServer(t.Context(), home, "http://127.0.0.1:1234", []string{"--model", "synthetic"}, ""); err != nil {
		t.Fatal(err)
	}
	pgid, err := strconv.Atoi(strings.TrimSpace(string(mustSessionCompanionFile(t, filepath.Join(root, "native-pgid")))))
	if err != nil {
		t.Fatalf("native process group = %v", err)
	}
	if pgid <= 0 || pgid == foregroundGroup {
		t.Fatalf("native process group = %d, foreground group = %d", pgid, foregroundGroup)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if connection, dialErr := net.DialTimeout("unix", filepath.Join(root, "native-live"), 50*time.Millisecond); dialErr != nil {
			break
		} else {
			_ = connection.Close()
		}
		time.Sleep(10 * time.Millisecond)
	}
	nativePID, err := readSessionCompanionPID(filepath.Join(root, "native-pid"))
	if err != nil {
		t.Fatal(err)
	}
	if sessionCompanionProcessAlive(nativePID) && waitForSessionNativeGone(filepath.Join(root, "native-pid")) == false {
		t.Fatalf("session-owned native descendant %d survived teardown", nativePID)
	}
	if err := syscall.Kill(unrelated.Process.Pid, 0); err != nil {
		t.Fatalf("unrelated process was terminated: %v", err)
	}
}

func stopSessionCompanionNative(root string) {
	pid, err := readSessionCompanionPID(filepath.Join(root, "native-pid"))
	if err == nil && pid > 0 && sessionCompanionProcessAlive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

func readSessionCompanionPID(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid session companion PID %q", content)
	}
	return pid, nil
}

func sessionCompanionProcessAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	state, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err == nil {
		fields := strings.Fields(string(state))
		return len(fields) < 3 || fields[2] != "Z"
	}
	return true
}

func TestRunThroughProxyWithSessionServerPassesFullAccessToCompanion(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	argsMarker := filepath.Join(root, "companion-args")
	wrapper := sessionCompanionWrapper(t)
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	t.Setenv(sessionCompanionArgsEnv, argsMarker)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(wrapper, Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	if err := process.RunThroughProxyWithSessionServer(t.Context(), home, "http://127.0.0.1:1234", []string{"--dangerously-bypass-approvals-and-sandbox", "--model", "synthetic"}, ""); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsMarker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-c\x00approval_policy=\"never\"") ||
		!strings.Contains(string(args), "-c\x00sandbox_mode=\"danger-full-access\"") {
		t.Fatalf("companion omitted full-access config: %q", args)
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

func sessionCompanionTreeWrapper(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex-wrapper")
	content := "#!/bin/sh\n\"$GODEX_TEST_SESSION_COMPANION_BINARY\" -test.run=^" + sessionCompanionHelperName + "$ -- \"$@\"\n"
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
