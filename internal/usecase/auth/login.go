package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	accountmodel "github.com/christiandoxa/godex/internal/model/account"
)

type loginAccounts interface {
	CreateStagedHome() (string, error)
	RemoveStagedHome(string) error
	CommitLogin(context.Context, accountentity.Account, string, bool) (accountentity.Account, error)
}

type loginCodex interface {
	Login(context.Context, string, bool) (accountentity.Identity, error)
}

type Login struct {
	accounts  loginAccounts
	codex     loginCodex
	now       func() time.Time
	removeAll func(string) error
}

func NewLogin(accounts loginAccounts, codex loginCodex) *Login {
	return &Login{accounts: accounts, codex: codex, now: time.Now, removeAll: accounts.RemoveStagedHome}
}

func (login *Login) Run(ctx context.Context, input accountmodel.LoginInput) (account accountentity.Account, err error) {
	stagedHome, err := login.accounts.CreateStagedHome()
	if err != nil {
		return accountentity.Account{}, err
	}
	defer func() {
		if cleanupErr := login.removeAll(stagedHome); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove staged Codex home: %w", cleanupErr))
		}
	}()

	identity, err := login.codex.Login(ctx, stagedHome, input.DeviceAuth)
	if err != nil {
		return accountentity.Account{}, err
	}
	candidate, err := accountentity.NewAccount(identity, input.Name, login.now())
	if err != nil {
		return accountentity.Account{}, err
	}
	return login.accounts.CommitLogin(ctx, candidate, stagedHome, input.Name != "")
}
