package runtime

import (
	"context"
	"errors"
	"strings"
)

type sharedSessionPreparer interface {
	PrepareSharedSessionHome(profileHome, sharedHome string) error
}

func (runner *Runner) prepareSharedAccountHomes(ctx context.Context) (err error) {
	if runner == nil || runner.accounts == nil || strings.TrimSpace(runner.sharedCodexHome) == "" {
		return nil
	}
	preparer, ok := runner.process.(sharedSessionPreparer)
	if !ok {
		return nil
	}
	accounts, err := runner.accounts.List(ctx)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		if account.ID != "" {
			ids = append(ids, account.ID)
		}
	}
	if leases, ok := runner.accounts.(interface {
		AcquireProfiles(context.Context, []string) (func() error, error)
	}); ok && len(ids) > 0 {
		release, acquireErr := leases.AcquireProfiles(ctx, ids)
		if acquireErr != nil {
			return acquireErr
		}
		defer func() { err = errors.Join(err, release()) }()
	}
	for _, account := range accounts {
		if account.ID == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := preparer.PrepareSharedSessionHome(runner.accounts.CodexHome(account.ID), runner.sharedCodexHome); err != nil {
			return err
		}
	}
	return nil
}
