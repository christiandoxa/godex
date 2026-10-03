package antigravity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Terminal struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type Process struct {
	binary   string
	terminal Terminal
}

func NewProcess(binary string, terminal Terminal) *Process {
	return &Process{binary: binary, terminal: terminal}
}

func (process *Process) PrepareCodexHome(codexHome string) error {
	if process == nil {
		return errors.New("antigravity CLI is not configured")
	}
	if codexHome == "" {
		return errors.New("antigravity CODEX_HOME is not configured")
	}
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return fmt.Errorf("create Antigravity CODEX_HOME: %w", err)
	}
	return nil
}

func (process *Process) RunWithCodexHome(ctx context.Context, codexHome string, arguments []string) error {
	return process.runWithCodexHome(ctx, codexHome, arguments, false)
}

func (process *Process) RunRuntimeWithCodexHome(ctx context.Context, codexHome string, arguments []string) error {
	return process.runWithCodexHome(ctx, codexHome, arguments, true)
}

func (process *Process) runWithCodexHome(ctx context.Context, codexHome string, arguments []string, clearRTKEnv bool) error {
	if err := process.PrepareCodexHome(codexHome); err != nil {
		return err
	}
	binary, err := process.resolveBinary()
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, binary, arguments...)
	// The child shares the terminal process group and receives its signal directly.
	command.Cancel = func() error { return nil }
	command.WaitDelay = 2 * time.Second
	command.Stdin = process.terminal.Stdin
	command.Stdout = process.terminal.Stdout
	command.Stderr = process.terminal.Stderr
	environment := os.Environ()
	command.Env = make([]string, 0, len(environment)+1)
	for _, variable := range environment {
		name, _, _ := strings.Cut(variable, "=")
		if equalEnvironmentName(name, "CODEX_HOME") {
			continue
		}
		if clearRTKEnv && (equalEnvironmentName(name, "PRODEX_RTK_AUTO_WRAP_DEPTH") ||
			equalEnvironmentName(name, "PRODEX_RTK_DISABLE_AUTO_WRAP")) {
			continue
		}
		command.Env = append(command.Env, variable)
	}
	command.Env = append(command.Env, "CODEX_HOME="+codexHome)
	if err := command.Run(); err != nil {
		var childError *exec.ExitError
		if ctx.Err() != nil && !errors.As(err, &childError) {
			return ctx.Err()
		}
		return fmt.Errorf("antigravity CLI failed: %w", err)
	}
	return nil
}

func equalEnvironmentName(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (process *Process) resolveBinary() (string, error) {
	candidate := process.binary
	if strings.TrimSpace(candidate) == "" {
		candidate = "agy"
	}
	binary, err := exec.LookPath(candidate)
	if err != nil {
		return "", fmt.Errorf("antigravity CLI %q was not found; install it or set PRODEX_AGY_BIN", candidate)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return "", fmt.Errorf("resolve antigravity CLI path: %w", err)
	}
	info, err := os.Stat(binary)
	if err != nil {
		return "", fmt.Errorf("inspect antigravity CLI %q: %w", candidate, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("antigravity CLI %q is not a regular executable", candidate)
	}
	return binary, nil
}
