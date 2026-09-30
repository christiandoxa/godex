package auth

import (
	"context"
	"errors"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	authmodel "github.com/christiandoxa/godex/internal/model/auth"
)

type nativeAccounts interface {
	Current(context.Context) (accountentity.Account, error)
	Resolve(context.Context, string) (accountentity.Account, error)
	CodexHome(string) string
	AcquireProfiles(context.Context, []string) (func() error, error)
	AcquireProfileMutation(context.Context, string) (func() error, error)
}
type nativeProcess interface {
	Run(context.Context, string, []string) error
}

type Native struct {
	accounts nativeAccounts
	process  nativeProcess
}

func NewNative(accounts nativeAccounts, process nativeProcess) *Native {
	return &Native{accounts, process}
}

func (native *Native) Run(ctx context.Context, input authmodel.Command) (err error) {
	var account accountentity.Account
	if input.Selector == "" {
		account, err = native.accounts.Current(ctx)
	} else {
		account, err = native.accounts.Resolve(ctx, input.Selector)
	}
	if err != nil {
		return err
	}
	var release func() error
	if input.Logout {
		release, err = native.accounts.AcquireProfileMutation(ctx, account.ID)
	} else {
		release, err = native.accounts.AcquireProfiles(ctx, []string{account.ID})
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	args := []string{"login", "status"}
	if input.Logout {
		args = []string{"logout"}
	}
	return native.process.Run(ctx, native.accounts.CodexHome(account.ID), args)
}
