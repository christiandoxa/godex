package runtime

import (
	"context"
	"errors"
	"fmt"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (runner *Runner) pinProfiles(ctx context.Context, ids []string) (func() error, error) {
	if leases, ok := runner.accounts.(interface {
		AcquireProfiles(context.Context, []string) (func() error, error)
	}); ok {
		return leases.AcquireProfiles(ctx, ids)
	}
	return func() error { return nil }, nil
}

// Leases prevent further mutations; refresh eligibility after acquiring them.
func (runner *Runner) pinnedAccounts(ctx context.Context, preferredID string, profiles []proxymodel.Account) ([]proxymodel.Account, error) {
	accounts, err := runner.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	enabled := make(map[string]bool, len(accounts))
	for _, account := range accounts {
		enabled[account.ID] = account.Enabled
	}
	if !enabled[preferredID] {
		return nil, errors.New("selected upstream account is unavailable; retry account selection")
	}
	profiles = append([]proxymodel.Account(nil), profiles...)
	for i := range profiles {
		profiles[i].Enabled = enabled[profiles[i].ID]
	}
	return profiles, nil
}

// The rollout home and upstream account can differ after precommit rotation.
func (runner *Runner) RunSession(ctx context.Context, homeID, ownerID string, args []string) error {
	return runner.RunSessionWithOptions(ctx, homeID, ownerID, args, RuntimeLaunchOptions{})
}

func (runner *Runner) RunSessionWithOptions(
	ctx context.Context,
	homeID, ownerID string,
	args []string,
	options RuntimeLaunchOptions,
) (err error) {
	if err := runner.prepareSharedAccountHomes(ctx); err != nil {
		return err
	}
	accounts, err := runner.accounts.List(ctx)
	if err != nil {
		return err
	}
	homeFound := false
	var owner *proxymodel.Account
	for _, account := range accounts {
		if account.ID == homeID {
			homeFound = true
		}
		if account.ID == ownerID && account.Enabled {
			owner = &proxymodel.Account{ID: account.ID, Home: runner.accounts.CodexHome(account.ID), Enabled: true}
		}
	}
	if !homeFound || owner == nil {
		return fmt.Errorf("session home or upstream owner is unavailable; continuity was preserved")
	}
	release, err := runner.pinProfiles(ctx, []string{homeID, ownerID})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	return runner.launchWithOptions(ctx, homeID, ownerID, []proxymodel.Account{*owner}, args, options)
}
