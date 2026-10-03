package antigravity

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const (
	helperEnabledEnv  = "GODEX_ANTIGRAVITY_HELPER"
	helperOutputEnv   = "GODEX_ANTIGRAVITY_ARGS"
	helperHomeEnv     = "GODEX_ANTIGRAVITY_CODEX_HOME"
	helperCheckRTKEnv = "GODEX_ANTIGRAVITY_CHECK_RTK_ENV"
	helperSignalEnv   = "GODEX_ANTIGRAVITY_WAIT_FOR_INTERRUPT"
)

func TestProcessRunsArgumentsWithoutShellAndPreservesExitCode(t *testing.T) {
	output := filepath.Join(t.TempDir(), "args.json")
	t.Setenv(helperEnabledEnv, "1")
	t.Setenv(helperOutputEnv, output)
	process := NewProcess(os.Args[0], Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	err := process.RunWithCodexHome(context.Background(), filepath.Join(t.TempDir(), "codex"), []string{
		"-test.run=^TestAntigravityProcessHelper$", "--", "auth login", "value;literal",
	})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("child error = %v", err)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatal(err)
	}
	if want := []string{"auth login", "value;literal"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("child arguments = %#v, want %#v", got, want)
	}
}

func TestProcessUsesAndCreatesSharedCodexHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "shared-codex")
	capturedHome := filepath.Join(t.TempDir(), "codex-home.txt")
	t.Setenv(helperEnabledEnv, "1")
	t.Setenv(helperOutputEnv, filepath.Join(t.TempDir(), "args.json"))
	t.Setenv(helperHomeEnv, capturedHome)
	t.Setenv("CODEX_HOME", "/synthetic/ambient")
	process := NewProcess(os.Args[0], Terminal{Stdout: io.Discard, Stderr: io.Discard})
	err := process.RunWithCodexHome(context.Background(), home, []string{
		"-test.run=^TestAntigravityProcessHelper$", "--", "capture-home",
	})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("child error = %v", err)
	}
	got, err := os.ReadFile(capturedHome)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != home {
		t.Fatalf("child CODEX_HOME = %q, want %q", got, home)
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		t.Fatalf("shared CODEX_HOME directory = %#v, err=%v", info, err)
	}
}

func TestProcessRemovesRtkAutoWrapControlEnvironment(t *testing.T) {
	t.Setenv(helperEnabledEnv, "1")
	t.Setenv(helperOutputEnv, filepath.Join(t.TempDir(), "args.json"))
	t.Setenv(helperCheckRTKEnv, "clear")
	t.Setenv("PRODEX_RTK_AUTO_WRAP_DEPTH", "synthetic-depth")
	t.Setenv("PRODEX_RTK_DISABLE_AUTO_WRAP", "1")
	if runtime.GOOS != "windows" {
		t.Setenv("prodex_rtk_auto_wrap_depth", "preserve-lowercase")
	}
	process := NewProcess(os.Args[0], Terminal{Stdout: io.Discard, Stderr: io.Discard})
	err := process.RunRuntimeWithCodexHome(context.Background(), filepath.Join(t.TempDir(), "codex"), []string{
		"-test.run=^TestAntigravityProcessHelper$", "--",
	})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("child with cleared RTK environment error = %v", err)
	}
}

func TestProcessLoginPreservesRtkAutoWrapControlEnvironment(t *testing.T) {
	t.Setenv(helperEnabledEnv, "1")
	t.Setenv(helperOutputEnv, filepath.Join(t.TempDir(), "args.json"))
	t.Setenv(helperCheckRTKEnv, "preserve")
	t.Setenv("PRODEX_RTK_AUTO_WRAP_DEPTH", "synthetic-depth")
	t.Setenv("PRODEX_RTK_DISABLE_AUTO_WRAP", "1")
	process := NewProcess(os.Args[0], Terminal{Stdout: io.Discard, Stderr: io.Discard})
	err := process.RunWithCodexHome(context.Background(), filepath.Join(t.TempDir(), "codex"), []string{
		"-test.run=^TestAntigravityProcessHelper$", "--",
	})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("login child with inherited RTK environment error = %v", err)
	}
}

func TestProcessChildHandlesTerminalInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows process control uses console events")
	}
	output := filepath.Join(t.TempDir(), "signal.txt")
	t.Setenv(helperEnabledEnv, "1")
	t.Setenv(helperSignalEnv, output)
	reader, writer := io.Pipe()
	defer reader.Close()
	process := NewProcess(os.Args[0], Terminal{Stdout: writer, Stderr: io.Discard})
	home := filepath.Join(t.TempDir(), "codex")
	result := make(chan error, 1)
	go func() {
		result <- process.RunWithCodexHome(context.Background(), home, []string{
			"-test.run=^TestAntigravityProcessHelper$", "--",
		})
	}()
	var pid int
	if _, err := fmt.Fscanf(bufio.NewReader(reader), "ready %d\n", &pid); err != nil || pid <= 0 {
		t.Fatalf("child readiness = %d, err=%v", pid, err)
	}
	child, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err = <-result
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
		t.Fatalf("interrupted child error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil || string(got) != "interrupt" {
		t.Fatalf("child signal = %q, err=%v", got, err)
	}
}

func TestProcessPreparesSharedCodexHomeWithoutResolvingBinary(t *testing.T) {
	home := filepath.Join(t.TempDir(), "shared-codex")
	process := NewProcess(filepath.Join(t.TempDir(), "missing-agy"), Terminal{})
	if err := process.PrepareCodexHome(home); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		t.Fatalf("prepared CODEX_HOME = %#v, err=%v", info, err)
	}
}

func TestAntigravityProcessHelper(t *testing.T) {
	if os.Getenv(helperEnabledEnv) != "1" {
		return
	}
	switch os.Getenv(helperCheckRTKEnv) {
	case "clear":
		if os.Getenv("PRODEX_RTK_AUTO_WRAP_DEPTH") != "" || os.Getenv("PRODEX_RTK_DISABLE_AUTO_WRAP") != "" {
			os.Exit(24)
		}
		if runtime.GOOS != "windows" && os.Getenv("prodex_rtk_auto_wrap_depth") != "preserve-lowercase" {
			os.Exit(26)
		}
	case "preserve":
		if os.Getenv("PRODEX_RTK_AUTO_WRAP_DEPTH") != "synthetic-depth" || os.Getenv("PRODEX_RTK_DISABLE_AUTO_WRAP") != "1" {
			os.Exit(25)
		}
	}
	if path := os.Getenv(helperSignalEnv); path != "" {
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)
		fmt.Fprintf(os.Stdout, "ready %d\n", os.Getpid())
		<-interrupts
		if err := os.WriteFile(path, []byte("interrupt"), 0o600); err != nil {
			os.Exit(2)
		}
		os.Exit(130)
	}
	var arguments []string
	for index, argument := range os.Args {
		if argument == "--" {
			arguments = os.Args[index+1:]
			break
		}
	}
	content, err := json.Marshal(arguments)
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv(helperOutputEnv), content, 0o600); err != nil {
		os.Exit(2)
	}
	if path := os.Getenv(helperHomeEnv); path != "" {
		if err := os.WriteFile(path, []byte(os.Getenv("CODEX_HOME")), 0o600); err != nil {
			os.Exit(2)
		}
	}
	os.Exit(23)
}

func TestProcessRejectsMissingAndNonFileBinaries(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "agy-missing")
	if _, err := NewProcess(missing, Terminal{}).resolveBinary(); err == nil {
		t.Fatal("missing Antigravity CLI unexpectedly resolved")
	}
	if _, err := NewProcess(t.TempDir(), Terminal{}).resolveBinary(); err == nil {
		t.Fatal("directory unexpectedly resolved as Antigravity CLI")
	}
}

func TestProcessHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	process := NewProcess(os.Args[0], Terminal{Stdout: io.Discard, Stderr: io.Discard})
	if err := process.RunWithCodexHome(ctx, t.TempDir(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled process error = %v", err)
	}
}
