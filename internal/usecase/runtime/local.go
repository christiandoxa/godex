package runtime

import (
	"context"
	"errors"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type activeAccounts interface {
	Current(context.Context) (accountentity.Account, error)
	Resolve(context.Context, string) (accountentity.Account, error)
}

func (runner *Runner) activeAccount(ctx context.Context, selector string) (accountentity.Account, error) {
	accounts, ok := runner.accounts.(activeAccounts)
	if !ok {
		return accountentity.Account{}, errors.New("active account lookup is not configured")
	}
	if selector != "" {
		return accounts.Resolve(ctx, selector)
	}
	return accounts.Current(ctx)
}

// Local native commands neither consume quota nor advance account rotation.
func (runner *Runner) RunLocal(ctx context.Context, selector string, args []string) (err error) {
	if err := runner.prepareSharedAccountHomes(ctx); err != nil {
		return err
	}
	account, err := runner.activeAccount(ctx, selector)
	if err != nil {
		return err
	}
	if leases, ok := runner.accounts.(interface {
		AcquireProfiles(context.Context, []string) (func() error, error)
	}); ok {
		release, err := leases.AcquireProfiles(ctx, []string{account.ID})
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, release()) }()
	}
	return runner.runRuntimeChild(ctx, runner.accounts.CodexHome(account.ID), args)
}

func (runner *Runner) RunCurrent(ctx context.Context, selector string, args []string) (err error) {
	if err := runner.prepareSharedAccountHomes(ctx); err != nil {
		return err
	}
	account, err := runner.activeAccount(ctx, selector)
	if err != nil {
		return err
	}
	if !account.Enabled {
		return errors.New("selected account is disabled")
	}
	profiles, err := runner.proxyAccounts(ctx, nil, selector, account.ID)
	if err != nil {
		return err
	}
	ids := []string{account.ID}
	for _, profile := range profiles {
		ids = append(ids, profile.ID)
	}
	release, err := runner.pinProfiles(ctx, ids)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	return runner.launch(ctx, account.ID, account.ID, profiles, args)
}

func (runner *Runner) RunHome(ctx context.Context, codexHome string, args []string) error {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	return runner.runRuntimeChild(ctx, home, args)
}
