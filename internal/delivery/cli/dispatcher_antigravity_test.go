package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type antigravityDispatcherProcess struct {
	preparedHome string
	runs         int
}

func (process *antigravityDispatcherProcess) PrepareCodexHome(home string) error {
	process.preparedHome = home
	return nil
}

func (process *antigravityDispatcherProcess) RunRuntimeWithCodexHome(context.Context, string, []string) error {
	process.runs++
	return nil
}

func TestAntigravityDispatcherSkipsUpdateNoticeForNativeLaunch(t *testing.T) {
	for _, arguments := range [][]string{
		{"run", "--provider", "gemini", "--cli", "agy", "exec"},
		{"run", "session-id", "--provider", "gemini", "--cli", "agy"},
		{"run", "--provider", "gemini", "--cli", "agy", "--dry-run"},
	} {
		if shouldShowUpdateNotice(arguments) {
			t.Fatalf("native Antigravity arguments %#v unexpectedly show update notice", arguments)
		}
	}
	if !shouldShowUpdateNotice([]string{"login", "--with-antigravity"}) {
		t.Fatal("Antigravity login unexpectedly skipped normal update-notice policy")
	}
}

func TestAntigravityDispatcherDryRunUsesPreparedHomeWithoutLaunch(t *testing.T) {
	process := &antigravityDispatcherProcess{}
	runner := runtimeusecase.NewRunner(nil, nil, nil)
	runner.SetAntigravityProcess(process)
	home := t.TempDir()
	runner.SetAntigravityCodexHome(home)
	var output bytes.Buffer
	app := New(nil, nil, nil, runner, nil, nil, &output)
	if err := app.Run(context.Background(), []string{
		"run", "session-id", "--provider", "gemini", "--cli", "agy", "--dry-run",
	}); err != nil {
		t.Fatal(err)
	}
	if process.preparedHome != home || process.runs != 0 {
		t.Fatalf("dry-run prepared %q and launched %d times", process.preparedHome, process.runs)
	}
	if !strings.Contains(output.String(), "Provider: antigravity") {
		t.Fatalf("dry-run output = %q", output.String())
	}
}

func TestHelpDocumentsNativeAntigravityRuntimeAndLoginCheckpoint(t *testing.T) {
	var output bytes.Buffer
	if err := printHelp(&output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--provider gemini --cli agy", "--with-antigravity"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("help missing %q", expected)
		}
	}
}
