package runtime

import (
	"context"
	"errors"
	"sort"
	"strings"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
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
	accounts = append([]accountentity.Account(nil), accounts...)
	sort.SliceStable(accounts, func(i, j int) bool {
		if !accounts[i].CreatedAt.Equal(accounts[j].CreatedAt) {
			if accounts[i].CreatedAt.IsZero() {
				return false
			}
			if accounts[j].CreatedAt.IsZero() {
				return true
			}
			return accounts[i].CreatedAt.Before(accounts[j].CreatedAt)
		}
		return accounts[i].ID < accounts[j].ID
	})
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
