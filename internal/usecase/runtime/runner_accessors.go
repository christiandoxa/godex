package runtime

import (
	"context"
	"errors"
)

func (runner *Runner) ProviderAPIKeys(provider, explicit string) ([]string, error) {
	if runner.credentials == nil {
		return nil, errors.New("runtime provider credential resolver is not configured")
	}
	return runner.credentials.APIKeys(provider, explicit)
}

func (runner *Runner) CurrentCodexHome() string { return runner.currentHome }

func (runner *Runner) Run(ctx context.Context, selector string, arguments []string) error {
	return runner.RunWithOptions(ctx, selector, arguments, RuntimeLaunchOptions{})
}
