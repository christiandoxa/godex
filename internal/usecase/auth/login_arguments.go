package auth

import (
	"context"
	"errors"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	accountmodel "github.com/christiandoxa/godex/internal/model/account"
)

type loginCodexArguments interface {
	LoginArguments(context.Context, string, []string) (accountentity.Identity, error)
}

func (login *Login) RunArguments(
	ctx context.Context,
	input accountmodel.LoginInput,
	arguments []string,
) (account accountentity.Account, err error) {
	stagedHome, err := login.accounts.CreateStagedHome()
	if err != nil {
		return accountentity.Account{}, err
	}
	defer func() {
		if cleanupErr := login.removeAll(stagedHome); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()

	var identity accountentity.Identity
	if runner, ok := login.codex.(loginCodexArguments); ok {
		identity, err = runner.LoginArguments(ctx, stagedHome, append([]string(nil), arguments...))
	} else if len(arguments) == 0 {
		identity, err = login.codex.Login(ctx, stagedHome, false)
	} else if len(arguments) == 1 && arguments[0] == "--device-auth" {
		identity, err = login.codex.Login(ctx, stagedHome, true)
	} else {
		return accountentity.Account{}, errors.New("Codex login passthrough support is not configured")
	}
	if err != nil {
		return accountentity.Account{}, err
	}
	candidate, err := accountentity.NewAccount(identity, input.Name, login.now())
	if err != nil {
		return accountentity.Account{}, err
	}
	return login.accounts.CommitLogin(ctx, candidate, stagedHome, input.Name != "")
}
