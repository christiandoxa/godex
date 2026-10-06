package runtime

import "context"

type runtimeChildProcess interface {
	RunRuntime(context.Context, string, []string) error
}

func (runner *Runner) runRuntimeChild(ctx context.Context, home string, arguments []string) error {
	if process, ok := runner.process.(runtimeChildProcess); ok {
		return process.RunRuntime(ctx, home, arguments)
	}
	return runner.process.Run(ctx, home, arguments)
}
