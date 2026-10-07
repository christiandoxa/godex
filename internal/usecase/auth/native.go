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

type antigravityProcess interface {
	RunWithCodexHome(context.Context, string, []string) error
}

type codexSessionLocker interface {
	LockCodexSessionsForChild(context.Context, string) (func() error, error)
}

type Native struct {
	accounts        nativeAccounts
	process         nativeProcess
	antigravity     antigravityProcess
	antigravityHome string
	sessionLocker   codexSessionLocker
}

func NewNative(accounts nativeAccounts, process nativeProcess, antigravity antigravityProcess) *Native {
	return &Native{accounts: accounts, process: process, antigravity: antigravity}
}

func (native *Native) SetAntigravityCodexHome(home string) {
	native.antigravityHome = home
}

func (native *Native) SetAntigravitySessionLocker(locker codexSessionLocker) {
	native.sessionLocker = locker
}

func (native *Native) RunAntigravityLogin(ctx context.Context) (runErr error) {
	if native == nil || native.antigravity == nil {
		return errors.New("antigravity CLI is not configured")
	}
	if native.antigravityHome == "" {
		return errors.New("antigravity CODEX_HOME is not configured")
	}
	if native.sessionLocker == nil {
		return errors.New("antigravity session lock is not configured")
	}
	release, err := native.sessionLocker.LockCodexSessionsForChild(ctx, native.antigravityHome)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	return native.antigravity.RunWithCodexHome(ctx, native.antigravityHome, []string{"auth", "login"})
}

func (native *Native) RunHome(ctx context.Context, home string, arguments []string) error {
	if native == nil || native.process == nil {
		return errors.New("native authentication commands are not configured")
	}
	if home == "" {
		return errors.New("selected profile CODEX_HOME is unavailable")
	}
	return native.process.Run(ctx, home, arguments)
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
