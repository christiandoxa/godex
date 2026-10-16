//go:build unix

package codex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const (
	codexSignalHelperEnv  = "GODEX_TEST_CODEX_SIGNAL_HELPER"
	codexSignalMarkerEnv  = "GODEX_TEST_CODEX_SIGNAL_MARKER"
	codexSignalMarkerText = "handled"
	codexCancelHelperEnv  = "GODEX_TEST_CODEX_CANCEL_HELPER"
	codexCancelMarkerEnv  = "GODEX_TEST_CODEX_CANCEL_MARKER"
	codexCancelMarkerText = "terminated"
)

func TestCodexSignalChildHelper(t *testing.T) {
	if os.Getenv(codexSignalHelperEnv) != "1" {
		return
	}
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	fmt.Fprintf(os.Stdout, "ready %d\n", os.Getpid())
	<-interrupts
	fmt.Fprintln(os.Stdout, "received")
	<-interrupts
	if err := os.WriteFile(os.Getenv(codexSignalMarkerEnv), []byte(codexSignalMarkerText), 0o600); err != nil {
		os.Exit(2)
	}
	os.Exit(130)
}

func TestRunCancellationSendsTermination(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "terminated")
	t.Setenv(codexCancelHelperEnv, "1")
	t.Setenv(codexCancelMarkerEnv, marker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	home := t.TempDir()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	process := NewCodexProcess(os.Args[0], Terminal{Stdout: writer, Stderr: io.Discard})
	finished := make(chan error, 1)
	go func() {
		finished <- process.Run(ctx, home, []string{"-test.run=^TestCodexCancellationChildHelper$"})
	}()

	output := bufio.NewReader(reader)
	ready, err := output.ReadString('\n')
	if err != nil {
		t.Fatalf("read child readiness: %v", err)
	}
	if ready != "ready\n" {
		t.Fatalf("child readiness = %q", ready)
	}
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != codexCancelMarkerText {
		t.Fatalf("child termination handling = %q, err=%v", got, err)
	}
}

func TestCodexCancellationChildHelper(t *testing.T) {
	if os.Getenv(codexCancelHelperEnv) != "1" {
		return
	}
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, syscall.SIGTERM)
	defer signal.Stop(interrupts)
	fmt.Fprintln(os.Stdout, "ready")
	<-interrupts
	if err := os.WriteFile(os.Getenv(codexCancelMarkerEnv), []byte(codexCancelMarkerText), 0o600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestTerminalCommandCancelsNonTTYChildWithTermination(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "cancelled")
	t.Setenv(codexCancelHelperEnv, "1")
	t.Setenv(codexCancelMarkerEnv, marker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	command := terminalCommand(ctx, os.Args[0], []string{"-test.run=^TestCodexCancellationChildHelper$"})
	command.Env = append(os.Environ(), codexCancelHelperEnv+"=1", codexCancelMarkerEnv+"="+marker)
	command.Stdin = strings.NewReader("")
	command.Stdout = writer
	command.Stderr = io.Discard
	output := bufio.NewReader(reader)
	finished := make(chan error, 1)
	go func() { finished <- runTerminalCommand(ctx, command) }()
	if ready, err := output.ReadString('\n'); err != nil || ready != "ready\n" {
		t.Fatalf("child readiness = %q, err=%v", ready, err)
	}
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("child cancellation = %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != codexCancelMarkerText {
		t.Fatalf("child interrupt marker = %q, err=%v", got, err)
	}
}

func TestRunCancellationReapsNonTTYDescendants(t *testing.T) {
	root := t.TempDir()
	defer stopSessionCompanionNative(root)
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionTreeEnv, "1")
	marker := filepath.Join(root, "events")
	t.Setenv(sessionCompanionMarkerEnv, marker)
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
	process := NewCodexProcess(sessionCompanionTreeWrapper(t), Terminal{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
	})
	finished := make(chan error, 1)
	go func() {
		finished <- process.Run(ctx, home, []string{"app-server", "--listen", "unix://" + filepath.Join(root, "outer.sock")})
	}()
	if !waitForSessionForegroundFile(filepath.Join(root, "native-ready")) {
		t.Fatal("direct child fixture did not publish native readiness")
	}
	nativePID, err := readSessionCompanionPID(filepath.Join(root, "native-pid"))
	if err != nil || !sessionCompanionProcessAlive(nativePID) {
		t.Fatalf("native descendant %d was not live: %v", nativePID, err)
	}
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if !waitForSessionNativeGone(filepath.Join(root, "native-pid")) {
		t.Fatal("non-TTY child descendant survived cancellation")
	}
	if err := syscall.Kill(unrelated.Process.Pid, 0); err != nil {
		t.Fatalf("unrelated process was terminated: %v", err)
	}
}
