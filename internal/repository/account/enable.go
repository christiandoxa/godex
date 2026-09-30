package account

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

func (store *FileStore) SetEnabled(ctx context.Context, selector string, enabled bool) (updated accountentity.Account, err error) {
	err = store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		index, err := resolveIndex(state.Accounts, selector)
		if err != nil {
			return err
		}
		// Do not change enablement while a child owns its launch eligibility snapshot.
		release, err := store.acquireProfile(state.Accounts[index].ID)
		if err != nil {
			return err
		}
		defer release()
		nextID := cursorAccountID(state)
		state.Accounts[index].Enabled = enabled
		state.Accounts[index].UpdatedAt = store.now().UTC()
		updated = state.Accounts[index]
		if !enabled {
			repairSelectionAfterRemove(&state, updated.ID, nextID)
		} else {
			if state.ActiveAccountID == "" {
				state.ActiveAccountID = updated.ID
			}
			if nextID != "" {
				state.RotationCursor = accountPosition(state.Accounts, nextID)
			}
		}
		_, err = store.writeState(state)
		return err
	})
	return updated, err
}
