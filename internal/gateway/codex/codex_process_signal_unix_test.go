//go:build unix

package codex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	codexSignalHelperEnv  = "GODEX_TEST_CODEX_SIGNAL_HELPER"
	codexSignalMarkerEnv  = "GODEX_TEST_CODEX_SIGNAL_MARKER"
	codexSignalMarkerText = "handled"
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

func TestRunLetsCodexHandleTerminalInterrupt(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "handled")
	t.Setenv(codexSignalHelperEnv, "1")
	t.Setenv(codexSignalMarkerEnv, marker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	home := t.TempDir()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	process := NewCodexProcess(os.Args[0], Terminal{Stdout: writer, Stderr: io.Discard})
	finished := make(chan error, 1)
	go func() {
		finished <- process.Run(ctx, home, []string{"-test.run=^TestCodexSignalChildHelper$"})
	}()

	output := bufio.NewReader(reader)
	ready, err := output.ReadString('\n')
	if err != nil {
		t.Fatalf("read child readiness: %v", err)
	}
	fields := strings.Fields(ready)
	if len(fields) != 2 || fields[0] != "ready" {
		t.Fatalf("child readiness = %q", ready)
	}
	pid, err := strconv.Atoi(fields[1])
	if err != nil {
		t.Fatalf("parse child pid: %v", err)
	}
	child, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Signal(os.Interrupt); err != nil {
		t.Fatalf("send terminal interrupt: %v", err)
	}
	received, err := output.ReadString('\n')
	if err != nil || received != "received\n" {
		t.Fatalf("child interrupt acknowledgement = %q, err=%v", received, err)
	}
	cancel()
	if err := child.Signal(os.Interrupt); err != nil {
		t.Fatalf("send follow-up interrupt after cancellation: %v", err)
	}
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != codexSignalMarkerText {
		t.Fatalf("child signal handling = %q, err=%v", got, err)
	}
}
