package codex

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	sessionLockHelperMode = "GODEX_TEST_SESSION_LOCK_MODE"
	sessionLockHelperPath = "GODEX_TEST_SESSION_LOCK_PATH"
)

func TestCodexSessionLockHelperProcess(t *testing.T) {
	mode := os.Getenv(sessionLockHelperMode)
	if mode == "" {
		return
	}
	switch mode {
	case "hold-exclusive":
		release, err := lockfile.TryAcquire(os.Getenv(sessionLockHelperPath))
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		_, _ = io.WriteString(os.Stdout, "locked\n")
		_, _ = io.Copy(io.Discard, os.Stdin)
	case "hold-shared":
		release, err := lockfile.TryRead(os.Getenv(sessionLockHelperPath))
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		_, _ = io.WriteString(os.Stdout, "locked\n")
		_, _ = io.Copy(io.Discard, os.Stdin)
	case "try-exclusive":
		release, err := lockfile.TryAcquire(os.Getenv(sessionLockHelperPath))
		if errors.Is(err, lockfile.ErrBusy) {
			_, _ = io.WriteString(os.Stdout, "busy\n")
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		_, _ = io.WriteString(os.Stdout, "acquired\n")
	case "child":
		_, _ = io.WriteString(os.Stdout, "ready\n")
		_, _ = io.Copy(io.Discard, os.Stdin)
	default:
		t.Fatalf("unknown session lock helper mode %q", mode)
	}
}

func TestCodexSessionLockerUsesProdexPathAndWaitsForCrossProcessLock(t *testing.T) {
	home := filepath.Join(t.TempDir(), "shared-codex")
	sessions := filepath.Join(home, "sessions")
	lockPath := filepath.Join(sessions, ".prodex-maintenance.lock")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	command, input := startSessionLockHelper(t, "hold-exclusive", lockPath)
	defer stopSessionLockHelper(command, input)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := (SessionLocker{}).LockCodexSessionsForChild(ctx, home); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled lock error = %v", err)
	}

	started := time.Now()
	_, err := (SessionLocker{}).LockCodexSessionsForChild(context.Background(), home)
	if err == nil || !strings.Contains(err.Error(), "timed out after 3000 ms waiting for Codex session lock") {
		t.Fatalf("contended lock error = %v", err)
	}
	if elapsed := time.Since(started); elapsed < 2800*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("lock wait = %s, want approximately 3s", elapsed)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("Prodex lock path %q was not created: %v", lockPath, err)
	}
}

func TestSessionLockerMaintenanceSkipsWhileChildLockIsHeld(t *testing.T) {
	home := filepath.Join(t.TempDir(), "shared-codex")
	sessions := filepath.Join(home, "sessions")
	lockPath := filepath.Join(sessions, ".prodex-maintenance.lock")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	command, input := startSessionLockHelper(t, "hold-shared", lockPath)
	defer stopSessionLockHelper(command, input)

	release, acquired, err := (SessionLocker{}).TryLockCodexSessionsForMaintenance(home)
	if err != nil {
		t.Fatal(err)
	}
	if acquired || release != nil {
		t.Fatalf("maintenance lock = acquired:%t release:%v, want skipped", acquired, release != nil)
	}
}

func TestSessionLockerMaintenanceExcludesChildAndReleases(t *testing.T) {
	home := filepath.Join(t.TempDir(), "shared-codex")
	release, acquired, err := (SessionLocker{}).TryLockCodexSessionsForMaintenance(home)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired || release == nil {
		t.Fatalf("maintenance lock = acquired:%t release:%v", acquired, release != nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := (SessionLocker{}).LockCodexSessionsForChild(ctx, home); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("child entered during maintenance: %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	childRelease, err := (SessionLocker{}).LockCodexSessionsForChild(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	if err := childRelease(); err != nil {
		t.Fatal(err)
	}
}

func TestCodexProcessHoldsSharedSessionLockUntilChildExits(t *testing.T) {
	home := filepath.Join(t.TempDir(), "shared-codex")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionLockHelperMode, "child")
	process := NewCodexProcess(os.Args[0], Terminal{Stdin: inputReader, Stdout: outputWriter, Stderr: io.Discard})
	runDone := make(chan error, 1)
	go func() {
		runDone <- process.Run(context.Background(), home, []string{"-test.run=^TestCodexSessionLockHelperProcess$"})
	}()
	defer func() {
		_ = inputWriter.Close()
		_ = inputReader.Close()
		_ = outputReader.Close()
		_ = outputWriter.Close()
	}()
	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(outputReader).ReadString('\n')
		ready <- line
	}()
	select {
	case line := <-ready:
		if line != "ready\n" {
			t.Fatalf("child startup output = %q", line)
		}
	case <-time.After(5 * time.Second):
		_ = inputWriter.Close()
		select {
		case err := <-runDone:
			t.Fatalf("Codex child did not start: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Codex child did not start or stop")
		}
	}
	lockPath := filepath.Join(home, "sessions", ".prodex-maintenance.lock")
	if got := runSessionLockProbe(t, "try-exclusive", lockPath); got != "busy" {
		t.Fatalf("exclusive probe while child runs = %q, want busy", got)
	}
	if err := inputWriter.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Codex child did not exit")
	}
	if got := runSessionLockProbe(t, "try-exclusive", lockPath); got != "acquired" {
		t.Fatalf("exclusive probe after child exits = %q, want acquired", got)
	}
}

func startSessionLockHelper(t *testing.T, mode, path string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	command, input, output := sessionLockHelperCommand(t, mode, path)
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "locked\n" {
		rest, _ := io.ReadAll(output)
		_ = input.Close()
		_ = command.Wait()
		_ = output.Close()
		t.Fatalf("session lock helper startup = %q%q, err=%v", line, rest, err)
	}
	_ = output.Close()
	return command, input
}

func stopSessionLockHelper(command *exec.Cmd, input io.WriteCloser) {
	_ = input.Close()
	_ = command.Wait()
}

func runSessionLockProbe(t *testing.T, mode, path string) string {
	t.Helper()
	command, input, output := sessionLockHelperCommand(t, mode, path)
	_ = input.Close()
	result, err := io.ReadAll(output)
	_ = output.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(strings.SplitN(string(result), "\n", 2)[0])
}

func sessionLockHelperCommand(t *testing.T, mode, path string) (*exec.Cmd, io.WriteCloser, io.ReadCloser) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestCodexSessionLockHelperProcess$")
	command.Env = []string{sessionLockHelperMode + "=" + mode, sessionLockHelperPath + "=" + path}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	return command, input, output
}
