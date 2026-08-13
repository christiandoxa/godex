package account

import (
	"context"
	"errors"
	"fmt"
	"os"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

func (store *FileStore) Remove(ctx context.Context, selector string) (entity.Account, error) {
	var removed entity.Account
	err := store.withLock(ctx, func() error {
		var err error
		removed, err = store.removeLocked(selector)
		return err
	})
	return removed, err
}

func (store *FileStore) removeLocked(selector string) (entity.Account, error) {
	state, err := store.readState()
	if err != nil {
		return entity.Account{}, err
	}
	index, err := resolveIndex(state.Accounts, selector)
	if err != nil {
		return entity.Account{}, err
	}
	removed := state.Accounts[index]
	nextID := cursorAccountID(state)
	trash, restore, err := store.stageRemoval(removed.ID)
	if err != nil {
		return entity.Account{}, err
	}
	state.Accounts = append(state.Accounts[:index], state.Accounts[index+1:]...)
	repairSelectionAfterRemove(&state, removed.ID, nextID)
	if err := store.finishRemoval(state, trash, restore); err != nil {
		return removed, err
	}
	return removed, nil
}

func (store *FileStore) finishRemoval(state stateFile, trash string, restore func() error) error {
	committed, err := store.writeState(state)
	if err != nil {
		if committed {
			return err
		}
		if restoreErr := restore(); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore removed account profile: %w", restoreErr))
		}
		return err
	}
	if trash == "" {
		return nil
	}
	if err := os.RemoveAll(trash); err != nil {
		return fmt.Errorf("remove staged account profile: %w", err)
	}
	return nil
}
