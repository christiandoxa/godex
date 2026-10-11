//go:build !windows

package codex

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunThroughProxyWithSessionServerCancellationReapsForegroundTree(t *testing.T) {
	root := t.TempDir()
	defer stopSessionCompanionNative(root)
	marker := filepath.Join(root, "events")
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	t.Setenv(sessionCompanionForegroundTreeEnv, "1")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	process := NewCodexProcess(sessionCompanionWrapper(t), Terminal{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
	})
	finished := make(chan error, 1)
	go func() {
		finished <- process.RunThroughProxyWithSessionServer(ctx, home, "http://127.0.0.1:1234", []string{"--model", "synthetic"}, "")
	}()
	if !waitForSessionForegroundFile(filepath.Join(root, "native-ready")) {
		t.Fatal("session fixture did not publish native readiness")
	}
	pgid, err := strconv.Atoi(strings.TrimSpace(string(mustSessionCompanionFile(t, filepath.Join(root, "native-pgid")))))
	if err != nil {
		t.Fatalf("foreground process group = %v", err)
	}
	if pgid <= 0 || pgid == syscall.Getpgrp() {
		t.Fatalf("foreground process group = %d, parent group = %d", pgid, syscall.Getpgrp())
	}
	connection, err := net.DialTimeout("unix", filepath.Join(root, "native-live"), 50*time.Millisecond)
	if err != nil {
		t.Fatalf("foreground descendant was not live: %v", err)
	}
	_ = connection.Close()
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if !waitForSessionNativeGone(filepath.Join(root, "native-live")) {
		t.Fatal("session-owned foreground descendant survived cancellation")
	}
	if err := syscall.Kill(unrelated.Process.Pid, 0); err != nil {
		t.Fatalf("unrelated process was terminated: %v", err)
	}
}

func TestRunThroughProxyWithSessionServerReportsStartupFailure(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	t.Setenv(sessionCompanionStartupFailureEnv, "1")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(sessionCompanionWrapper(t), Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	err := process.RunThroughProxyWithSessionServer(context.Background(), home, "http://127.0.0.1:1234", []string{"--model", "synthetic"}, "")
	if err == nil || !strings.Contains(err.Error(), "companion exited before becoming ready") {
		t.Fatalf("startup failure = %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".s")); !os.IsNotExist(statErr) {
		t.Fatalf("private socket survived startup failure: %v", statErr)
	}
}

func TestRunThroughProxyWithSessionServerPreservesChildExitStatus(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	t.Setenv(sessionCompanionExitCodeEnv, "23")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(sessionCompanionWrapper(t), Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	err := process.RunThroughProxyWithSessionServer(context.Background(), home, "http://127.0.0.1:1234", []string{"--model", "synthetic"}, "")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("child exit = %v, want status 23", err)
	}
}

func TestRunThroughProxyWithSessionServerRejectsExplicitRemote(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(sessionCompanionWrapper(t), Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	err := process.RunThroughProxyWithSessionServer(context.Background(), home, "http://127.0.0.1:1234", []string{"--remote", "unix:///tmp/user-selected.sock"}, "")
	if err == nil || !strings.Contains(err.Error(), "cannot override Godex routing") {
		t.Fatalf("explicit remote result = %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("explicit remote started child: %v", statErr)
	}
}

func waitForSessionForegroundFile(path string) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func waitForSessionNativeGone(path string) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("unix", path, 50*time.Millisecond)
		if err != nil {
			return true
		}
		_ = connection.Close()
		time.Sleep(time.Millisecond)
	}
	return false
}
