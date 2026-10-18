package runtime

import (
	"context"
	"errors"
	"strings"
)

type directRuntimeProcess interface {
	RunRuntimeDirect(context.Context, string, []string, bool) error
}

// RunDirectProfileWithOptions runs Codex from a profile home without creating
// the Godex runtime proxy. OpenAI API-key profiles use this path when no
// OpenAI-compatible base URL rewrite is configured.
func (runner *Runner) RunDirectProfileWithOptions(
	ctx context.Context,
	codexHome string,
	args []string,
	options RuntimeLaunchOptions,
) error {
	if runner == nil || runner.process == nil {
		return errors.New("runtime process is not configured")
	}
	if err := runner.prepareSharedAccountHomes(ctx); err != nil {
		return err
	}
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	if preparer, ok := runner.process.(sharedSessionPreparer); ok && strings.TrimSpace(runner.sharedCodexHome) != "" {
		if err := preparer.PrepareSharedSessionHome(home, runner.sharedCodexHome); err != nil {
			return err
		}
	}
	if options.SuperOverlay {
		prepared, err := runner.trustedWorkspaceArguments(ctx, args)
		if err != nil {
			return err
		}
		args = prepared
	}
	if process, ok := runner.process.(directRuntimeProcess); ok {
		return process.RunRuntimeDirect(ctx, home, args, options.UpstreamNoProxy)
	}
	return runner.runRuntimeChild(ctx, home, args)
}
