package main

import (
	"context"
	"os"
	"os/exec"
	"testing"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
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

func TestRuntimeProviderHomePrefersSelectedAccount(t *testing.T) {
	config := proxyconfig.Config{
		PreferredAccount: "selected",
		Accounts: func(context.Context) ([]proxyconfig.Account, error) {
			return []proxyconfig.Account{
				{ID: "other", Home: "/profiles/other", Enabled: true},
				{ID: "selected", Home: "/profiles/selected", Enabled: true},
			}, nil
		},
	}
	home, err := runtimeProviderHome(config)
	if err != nil || home != "/profiles/selected" {
		t.Fatalf("home = %q, err = %v", home, err)
	}
}

func TestRuntimeProviderHomeUsesSingleProfileFallbackAndRejectsAmbiguity(t *testing.T) {
	config := proxyconfig.Config{
		Accounts: func(context.Context) ([]proxyconfig.Account, error) {
			return []proxyconfig.Account{{ID: "only", Home: "/profiles/only", Enabled: true}}, nil
		},
	}
	home, err := runtimeProviderHome(config)
	if err != nil || home != "/profiles/only" {
		t.Fatalf("single home = %q, err = %v", home, err)
	}
	config.Accounts = func(context.Context) ([]proxyconfig.Account, error) {
		return []proxyconfig.Account{
			{ID: "one", Home: "/profiles/one", Enabled: true},
			{ID: "two", Home: "/profiles/two", Enabled: true},
		}, nil
	}
	if _, err := runtimeProviderHome(config); err == nil {
		t.Fatal("ambiguous provider pool unexpectedly resolved a home")
	}
}
