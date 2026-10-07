package runtime

import (
	"context"
	"errors"
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
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	if process, ok := runner.process.(directRuntimeProcess); ok {
		return process.RunRuntimeDirect(ctx, home, args, options.UpstreamNoProxy)
	}
	return runner.runRuntimeChild(ctx, home, args)
}
