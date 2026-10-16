//go:build linux

package codex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	codexInteractiveHelperEnv  = "GODEX_TEST_CODEX_INTERACTIVE_HELPER"
	codexInteractiveMarkerEnv  = "GODEX_TEST_CODEX_INTERACTIVE_MARKER"
	codexInteractiveMarkerText = "terminated"
)

func TestCodexInteractiveTerminalHelper(t *testing.T) {
	if os.Getenv(codexInteractiveHelperEnv) != "1" {
		return
	}
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)
	fmt.Fprintf(os.Stdout, "ready %d\n", os.Getpid())
	if <-interrupts != os.Interrupt {
		return
	}
	fmt.Fprintln(os.Stdout, "received")
	if err := os.WriteFile(os.Getenv(codexInteractiveMarkerEnv), []byte((<-interrupts).String()), 0o600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestRunTerminalCommandPreservesInteractiveFirstInterrupt(t *testing.T) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	marker := filepath.Join(t.TempDir(), "signal")
	master, slave := testSessionOwnedPTY043711(t)
	command := terminalCommand(ctx, os.Args[0], []string{"-test.run=^TestCodexInteractiveTerminalHelper$"})
	command.Env = append(os.Environ(), codexInteractiveHelperEnv+"=1", codexInteractiveMarkerEnv+"="+marker)
	command.Stdin = slave
	command.Stdout = slave
	command.Stderr = slave
	privateProcessGroup := configureCodexSessionChildProcess(command)
	prepareTerminalCommandWait(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- waitForSessionChild(ctx, command, privateProcessGroup) }()

	output := bufio.NewReader(master)
	ready, err := output.ReadString('\n')
	if err != nil || !strings.HasPrefix(strings.TrimSpace(ready), "ready ") {
		t.Fatalf("child readiness = %q, err=%v", ready, err)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send first parent SIGINT: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("parent SIGINT did not cancel the launch context")
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("send first child SIGINT: %v", err)
	}
	received, err := output.ReadString('\n')
	if err != nil || strings.TrimSpace(received) != "received" {
		t.Fatalf("child interrupt acknowledgement = %q, err=%v", received, err)
	}
	select {
	case err := <-finished:
		t.Fatalf("child was forced down after first SIGINT: %v", err)
	case <-time.After(2100 * time.Millisecond):
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send second parent SIGINT: %v", err)
	}
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("interactive cancellation = %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != codexInteractiveMarkerText {
		t.Fatalf("second interrupt signal = %q, err=%v", got, err)
	}
}
