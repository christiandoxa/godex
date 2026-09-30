package account

import (
	"context"
	"errors"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

func (store *FileStore) Current(ctx context.Context) (entity.Account, error) {
	if err := ctx.Err(); err != nil {
		return entity.Account{}, err
	}
	state, err := store.readSnapshot(ctx)
	if err != nil {
		return entity.Account{}, err
	}
	if state.ActiveAccountID == "" {
		return entity.Account{}, errors.New("no active account; run `godex login` or `godex profile import-current`")
	}
	for _, account := range state.Accounts {
		if account.ID == state.ActiveAccountID {
			return account, nil
		}
	}
	return entity.Account{}, errors.New("active account metadata is inconsistent")
}
