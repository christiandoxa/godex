package main

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestExitCodePreservesChildStatus(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestExitCodeChild$")
	command.Env = append(os.Environ(), "GODEX_EXIT_CODE_HELPER=1")
	err := command.Run()
	if got := exitCode(context.Background(), err); got != 23 {
		t.Fatalf("exit code = %d, want 23", got)
	}
}

func TestExitCodeReturnsCancelStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := exitCode(ctx, context.Canceled); got != 130 {
		t.Fatalf("exit code = %d, want 130", got)
	}
}

func TestRunListsAccountsFromConfiguredHome(t *testing.T) {
	t.Setenv("GODEX_HOME", t.TempDir())
	t.Setenv("GODEX_CODEX_BIN", "/synthetic/codex")
	original := os.Args
	os.Args = []string{"godex", "accounts"}
	t.Cleanup(func() { os.Args = original })
	if got := run(); got != 0 {
		t.Fatalf("run exit code = %d", got)
	}
}

func TestRunMapsUnknownCommandToFailure(t *testing.T) {
	t.Setenv("GODEX_HOME", t.TempDir())
	original := os.Args
	os.Args = []string{"godex", "synthetic-unknown"}
	t.Cleanup(func() { os.Args = original })
	if got := run(); got != 1 {
		t.Fatalf("run exit code = %d", got)
	}
}

func TestExitCodeChild(t *testing.T) {
	if os.Getenv("GODEX_EXIT_CODE_HELPER") != "1" {
		return
	}
	os.Exit(23)
}
