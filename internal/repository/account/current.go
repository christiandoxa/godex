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

func (store *FileStore) ActiveID(ctx context.Context) (string, error) {
	state, err := store.readSnapshot(ctx)
	if err != nil {
		return "", err
	}
	return state.ActiveAccountID, nil
}

func (store *FileStore) ClearActive(ctx context.Context) error {
	return store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		state.ActiveAccountID = ""
		_, err = store.writeState(state)
		return err
	})
}
