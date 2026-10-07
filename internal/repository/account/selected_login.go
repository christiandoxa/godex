package account

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

func (store *FileStore) ApplySelectedLogin(
	ctx context.Context,
	accountID string,
	identity accountentity.Identity,
	authJSON []byte,
) (accountentity.Account, error) {
	mail := strings.TrimSpace(identity.Email)
	accountKey := strings.TrimSpace(identity.ChatGPTAccountID)
	if mail == "" && accountKey == "" {
		return accountentity.Account{}, errors.New("selected login did not expose a ChatGPT identity")
	}
	if len(authJSON) == 0 || len(authJSON) > 2<<20 {
		return accountentity.Account{}, errors.New("selected login authentication is invalid or too large")
	}
	staged, err := store.CreateStagedHome()
	if err != nil {
		return accountentity.Account{}, err
	}
	defer func() { _ = store.RemoveStagedHome(staged) }()
	if _, err := fileutil.AtomicWrite(filepath.Join(staged, authFileName), authJSON); err != nil {
		return accountentity.Account{}, err
	}

	var result accountentity.Account
	err = store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		index, err := resolveIndex(state.Accounts, accountID)
		if err != nil || state.Accounts[index].ID != accountID {
			return errors.New("selected login account is unavailable")
		}
		release, err := store.acquireProfile(accountID)
		if err != nil {
			return err
		}
		defer release()

		current := state.Accounts[index]
		current.Email = mail
		current.ChatGPTAccountID = accountKey
		current.UpdatedAt = maxTime(current.UpdatedAt, store.now().UTC())
		if err := accountentity.ValidateAccount(current); err != nil {
			return err
		}
		state.Accounts[index] = current

		authPath := filepath.Join(store.CodexHome(accountID), authFileName)
		backup, err := store.transactionPath(authPath, "backup")
		if err != nil {
			return err
		}
		if _, err := store.beginTransaction("auth", accountID, backup, state); err != nil {
			return err
		}
		backup, rollback, err := store.replaceAuthentication(accountID, staged, backup)
		if err != nil {
			return err
		}
		if err := store.persistLoginState(state, backup, rollback); err != nil {
			return err
		}
		if err := store.finishTransaction(); err != nil {
			return err
		}
		result = current
		return nil
	})
	return result, err
}
