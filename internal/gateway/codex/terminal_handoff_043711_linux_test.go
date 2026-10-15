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

func testChildTermios043711(t *testing.T, slave *os.File) *unix.Termios {
	t.Helper()
	attrs, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	return attrs
}

func TestProdex043711ChildTerminalHandoffPreservesQueuedInputAndProtocols(t *testing.T) {
	master, slave := testSessionOwnedPTY043711(t)
	original := testChildTermios043711(t, slave)
	release := captureOwnedChildTerminal043711(slave, slave)
	defer release()
	if _, err := term.MakeRaw(int(slave.Fd())); err != nil {
		t.Fatal(err)
	}
	if raw := testChildTermios043711(t, slave); raw.Lflag&unix.ICANON != 0 {
		t.Fatal("PTY fixture did not enter raw mode")
	}
	pending := []byte("queued-before-exit\n")
	if count, err := master.Write(pending); err != nil || count != len(pending) {
		t.Fatalf("enqueue raw terminal input: count=%d err=%v", count, err)
	}
	release()
	restored := testChildTermios043711(t, slave)
	if restored.Lflag != original.Lflag || restored.Iflag != original.Iflag ||
		restored.Oflag != original.Oflag {
		t.Fatalf("inherited PTY mode not restored: before=%+v after=%+v", original, restored)
	}
	_ = slave.SetReadDeadline(time.Now().Add(time.Second))
	readback := make([]byte, len(pending))
	if _, err := io.ReadFull(slave, readback); err != nil {
		t.Fatalf("restoring inherited terminal flushed pending input: %v", err)
	}
	if !bytes.Equal(readback, pending) {
		t.Fatalf("queued terminal input changed: %q, want %q", readback, pending)
	}
	output := sessionOwnedPTYContents043711(t, master, slave)
	for _, control := range []string{
		"\x1b[?1047l",
		"\x1b[?1000l", "\x1b[?1006l",
		"\x1b[?2004l", "\x1b[?1004l", "\x1b[?25h",
		"\x1b[<1u",
	} {
		if !strings.Contains(output, control) || strings.Count(output, control) != 1 {
			t.Fatalf("missing or duplicate terminal cleanup %q: %q", control, output)
		}
	}
}

func TestProdex043711NativeTUIRestoresTerminalAfterSuccessAndFailure(t *testing.T) {
	for _, exit := range []int{0, 23} {
		t.Run(fmt.Sprintf("exit-%d", exit), func(t *testing.T) {
			root := t.TempDir()
			binary := filepath.Join(root, "mock-native")
			script := fmt.Sprintf("#!/bin/sh\nstty raw -echo <&0\nexit %d\n", exit)
			if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			home := filepath.Join(root, "home")
			if err := os.MkdirAll(home, 0o700); err != nil {
				t.Fatal(err)
			}
			master, slave := testSessionOwnedPTY043711(t)
			original := testChildTermios043711(t, slave)
			process := NewCodexProcess(binary, Terminal{Stdin: slave, Stdout: slave, Stderr: slave})
			err := process.RunRuntime(t.Context(), home, []string{"--model", "fixture"})
			if exit == 0 && err != nil {
				t.Fatal(err)
			}
			if exit != 0 {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != exit {
					t.Fatalf("native exit status changed by cleanup: %v", err)
				}
			}
			got := testChildTermios043711(t, slave)
			if got.Lflag != original.Lflag || got.Iflag != original.Iflag ||
				got.Oflag != original.Oflag {
				t.Fatalf("native TUI leaked raw mode: before=%+v after=%+v", original, got)
			}
			output := sessionOwnedPTYContents043711(t, master, slave)
			for _, code := range []string{"\x1b[?1047l", "\x1b[?2004l", "\x1b[?1004l", "\x1b[?25h"} {
				if !strings.Contains(output, code) {
					t.Fatalf("TUI exit did not reset terminal mode %q: %q", code, output)
				}
			}
		})
	}
}

func TestProdex043711NonInteractiveChildNeverEmitsTerminalProtocols(t *testing.T) {
	var stdout bytes.Buffer
	cleanup := captureOwnedChildTerminal043711(strings.NewReader(""), &stdout)
	cleanup()
	cleanup()
	if stdout.Len() != 0 {
		t.Fatalf("noninteractive terminal cleanup leaked escape codes: %q", stdout.String())
	}
}

func TestProdex043711OwnedCompanionRestoresTTYBeforeResumeGuidance(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "events")
	t.Setenv(sessionCompanionHelperEnv, "1")
	t.Setenv(sessionCompanionMarkerEnv, marker)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	master, slave := testSessionOwnedPTY043711(t)
	original := testChildTermios043711(t, slave)
	process := NewCodexProcess(sessionCompanionWrapper(t), Terminal{
		Stdin: slave, Stdout: slave, Stderr: slave,
	})
	if err := process.RunThroughProxyWithSessionServer(t.Context(), home,
		"http://127.0.0.1:1234", []string{"--model", "synthetic"}, ""); err != nil {
		t.Fatal(err)
	}
	got := testChildTermios043711(t, slave)
	if got.Lflag != original.Lflag || got.Iflag != original.Iflag {
		t.Fatalf("companion exit did not restore parent's termios: %+v before %+v", got, original)
	}
	if _, err := os.Lstat(filepath.Join(home, ".s")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned socket leaked after companion exit: %v", err)
	}
	output := sessionOwnedPTYContents043711(t, master, slave)
	reset := strings.LastIndex(output, "\x1b[?25h")
	guidance := strings.Index(output, "Godex saved session")
	if reset < 0 || guidance < 0 || reset > guidance {
		t.Fatalf("saved-session guidance preceded terminal restoration: %q", output)
	}
}
