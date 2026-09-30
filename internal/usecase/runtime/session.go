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

// The rollout home and upstream account can differ after precommit rotation.
func (runner *Runner) RunSession(ctx context.Context, homeID, ownerID string, args []string) (err error) {
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
	return runner.launch(ctx, homeID, ownerID, []proxymodel.Account{*owner}, args)
}
