//go:build linux

package codex

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func testSessionOwnedPTY043711(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	if !term.IsTerminal(int(slave.Fd())) {
		t.Fatal("session companion test did not open a real terminal")
	}
	return master, slave
}

func sessionOwnedPTYContents043711(t *testing.T, master, slave *os.File) string {
	t.Helper()
	if err := slave.Close(); err != nil {
		t.Fatal(err)
	}
	_ = master.SetReadDeadline(time.Now().Add(2 * time.Second))
	var buffer bytes.Buffer
	payload := make([]byte, 2048)
	for {
		count, err := master.Read(payload)
		if count != 0 {
			_, _ = buffer.Write(payload[:count])
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, unix.EIO) {
				break
			}
			t.Fatalf("read test terminal: %v", err)
		}
	}
	return buffer.String()
}

func TestProdex043711SuccessfulSessionCompanionShowsResumeGuidanceAfterSocketClosed(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	master, slave := testSessionOwnedPTY043711(t)
	process := NewCodexProcess(sessionCompanionWrapper(t), Terminal{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: slave,
	})
	if err := process.RunThroughProxyWithSessionServer(t.Context(), home,
		"http://127.0.0.1:1234", []string{"--model", "synthetic"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".s")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned app-server socket survived successful child exit: %v", err)
	}
	output := sessionOwnedPTYContents043711(t, master, slave)
	for _, wanted := range []string{
		"Godex saved session",
		"This session's temporary app-server has stopped",
		"Resume with: godex session resume <ID>",
		"Do not reuse the temporary codex --remote socket",
		"Paused goals stay paused until /goal resume.",
	} {
		if !strings.Contains(output, wanted) {
			t.Fatalf("missing %q in interactive exit guidance: %q", wanted, output)
		}
	}
	if strings.Count(output, "Resume with:") != 1 {
		t.Fatalf("duplicate or missing session resume guidance: %q", output)
	}
}

func TestProdex043711SessionCompanionSuppressesGuidanceOnNonTTYAndFailure(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	process := NewCodexProcess(sessionCompanionWrapper(t), Terminal{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &stderr,
	})
	if err := process.RunThroughProxyWithSessionServer(t.Context(), home,
		"http://127.0.0.1:1234", []string{"--model", "synthetic"}, ""); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("noninteractive session emitted terminal guidance: %q", stderr.String())
	}

	master, slave := testSessionOwnedPTY043711(t)
	t.Setenv(sessionCompanionExitCodeEnv, "23")
	process = NewCodexProcess(sessionCompanionWrapper(t), Terminal{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: slave,
	})
	err := process.RunThroughProxyWithSessionServer(t.Context(), home,
		"http://127.0.0.1:1234", []string{"--model", "synthetic"}, "")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("child exit status not preserved: %v", err)
	}
	if output := sessionOwnedPTYContents043711(t, master, slave); strings.TrimSpace(output) != "" {
		t.Fatalf("failed child falsely advertised saved session: %q", output)
	}
}
