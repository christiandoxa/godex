package runtime

import (
	"context"
	"errors"
	"slices"
	"strings"
)

type antigravityProcess interface {
	PrepareCodexHome(string) error
	RunRuntimeWithCodexHome(context.Context, string, []string) error
}

type codexSessionLocker interface {
	LockCodexSessionsForChild(context.Context, string) (func() error, error)
}

func (runner *Runner) SetAntigravityProcess(process antigravityProcess) {
	runner.antigravity = process
}

func (runner *Runner) SetAntigravityCodexHome(home string) {
	runner.antigravityHome = home
}

func (runner *Runner) SetAntigravitySessionLocker(locker codexSessionLocker) {
	runner.sessionLocker = locker
}

func (runner *Runner) PrepareAntigravityCodexHome() error {
	if runner == nil || runner.antigravity == nil {
		return errors.New("antigravity runtime is not configured")
	}
	if runner.antigravityHome == "" {
		return errors.New("antigravity CODEX_HOME is not configured")
	}
	return runner.antigravity.PrepareCodexHome(runner.antigravityHome)
}

func (runner *Runner) RunAntigravity(ctx context.Context, model string, arguments []string) (runErr error) {
	if runner == nil || runner.antigravity == nil {
		return errors.New("antigravity runtime is not configured")
	}
	if runner.antigravityHome == "" {
		return errors.New("antigravity CODEX_HOME is not configured")
	}
	if runner.sessionLocker == nil {
		return errors.New("antigravity session lock is not configured")
	}
	release, err := runner.sessionLocker.LockCodexSessionsForChild(ctx, runner.antigravityHome)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	return runner.antigravity.RunRuntimeWithCodexHome(ctx, runner.antigravityHome, antigravityArguments(model, arguments))
}

func antigravityArguments(model string, arguments []string) []string {
	result := append([]string(nil), arguments...)
	if !slices.Contains(arguments, "--dangerously-skip-permissions") {
		result = append([]string{"--dangerously-skip-permissions"}, result...)
	}
	if model != "" && !hasModelArgument(arguments) {
		result = append([]string{"--model", model}, result...)
	}
	return result
}

func hasModelArgument(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--model" || argument == "-m" ||
			strings.HasPrefix(argument, "--model=") || strings.HasPrefix(argument, "-m=") {
			return true
		}
	}
	return false
}
