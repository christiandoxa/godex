package account

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

func (store *FileStore) CommitLogin(
	ctx context.Context,
	candidate entity.Account,
	stagedCodexHome string,
	rename bool,
) (entity.Account, error) {
	if err := entity.ValidateAccount(candidate); err != nil {
		return entity.Account{}, err
	}
	if err := store.Prepare(); err != nil {
		return entity.Account{}, err
	}
	if err := store.validateStagedHome(stagedCodexHome); err != nil {
		return entity.Account{}, err
	}

	var committed entity.Account
	err := store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		account, err := store.commitLoginStateLocked(state, candidate, stagedCodexHome, rename, false)
		if err != nil {
			return err
		}
		committed = account
		return nil
	})
	return committed, err
}

// CommitImportCurrent applies the copy-current profile contract under one state lock:
// the requested name must still be free, duplicate identity updates only auth, and
// the resulting profile becomes active in the same persisted transaction.
func (store *FileStore) CommitImportCurrent(
	ctx context.Context,
	candidate entity.Account,
	stagedCodexHome string,
) (entity.Account, error) {
	if err := entity.ValidateAccount(candidate); err != nil {
		return entity.Account{}, err
	}
	if err := store.Prepare(); err != nil {
		return entity.Account{}, err
	}
	if err := store.validateStagedHome(stagedCodexHome); err != nil {
		return entity.Account{}, err
	}

	var committed entity.Account
	err := store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		if nameInUse(state.Accounts, candidate.Name, -1) {
			return fmt.Errorf("profile %q already exists", candidate.Name)
		}
		account, err := store.commitLoginStateLocked(state, candidate, stagedCodexHome, false, true)
		if err != nil {
			return err
		}
		committed = account
		return nil
	})
	return committed, err
}

func (store *FileStore) commitLoginStateLocked(
	state stateFile,
	candidate entity.Account,
	stagedCodexHome string,
	rename bool,
	activate bool,
) (entity.Account, error) {
	nextID := cursorAccountID(state)
	hadEnabled := len(orderedEnabledIndexes(state.Accounts)) > 0
	candidate, existingIndex, err := mergeLoginCandidate(state.Accounts, candidate, rename)
	if err != nil {
		return entity.Account{}, err
	}
	if activate {
		candidate.Enabled = true
	}

	release, err := store.acquireProfile(candidate.ID)
	if err != nil {
		return entity.Account{}, err
	}
	defer release()
	updateLoginState(&state, candidate, existingIndex, nextID, hadEnabled)
	if activate {
		state.ActiveAccountID = candidate.ID
		state.RotationCursor = accountPosition(state.Accounts, candidate.ID)
	}
	kind, base := "profile", store.accountDir(candidate.ID)
	if existingIndex >= 0 {
		kind, base = "auth", store.CodexHome(candidate.ID)+string(os.PathSeparator)+"auth.json"
	}
	backup, err := store.transactionPath(base, "backup")
	if err != nil {
		return entity.Account{}, err
	}
	if _, err := store.beginTransaction(kind, candidate.ID, backup, state); err != nil {
		return entity.Account{}, err
	}
	var rollback func() error
	if existingIndex >= 0 {
		backup, rollback, err = store.replaceAuthentication(candidate.ID, stagedCodexHome, backup)
	} else {
		backup, rollback, err = store.replaceProfile(candidate.ID, stagedCodexHome, backup)
	}
	if err != nil {
		return entity.Account{}, err
	}
	if err := store.persistLoginState(state, backup, rollback); err != nil {
		return entity.Account{}, err
	}
	if err := store.finishTransaction(); err != nil {
		return entity.Account{}, err
	}
	return candidate, nil
}

func mergeLoginCandidate(accounts []entity.Account, candidate entity.Account, rename bool) (entity.Account, int, error) {
	existingIndex := identityIndex(accounts, candidate)
	if existingIndex == -2 {
		return entity.Account{}, 0, errors.New("ChatGPT account identity is ambiguous; login must expose a ChatGPT account ID")
	}
	if existingIndex >= 0 {
		existing := accounts[existingIndex]
		candidate.ID = existing.ID
		if strings.TrimSpace(candidate.Email) == "" {
			candidate.Email = existing.Email
		}
		if strings.TrimSpace(candidate.ChatGPTAccountID) == "" {
			candidate.ChatGPTAccountID = existing.ChatGPTAccountID
		}
		candidate.CreatedAt = existing.CreatedAt
		candidate.Enabled = existing.Enabled
		candidate.UpdatedAt = maxTime(existing.UpdatedAt, candidate.UpdatedAt)
		candidate.LastUsedAt = existing.LastUsedAt
		if !rename {
			candidate.Name = existing.Name
		}
	} else if !rename {
		candidate.Name = availableDefaultName(accounts, candidate.Name)
	}
	if err := ensureUniqueName(accounts, candidate, existingIndex); err != nil {
		return entity.Account{}, 0, err
	}
	if err := entity.ValidateAccount(candidate); err != nil {
		return entity.Account{}, 0, err
	}
	return candidate, existingIndex, nil
}

func updateLoginState(state *stateFile, candidate entity.Account, existingIndex int, nextID string, hadEnabled bool) {
	if existingIndex >= 0 {
		state.Accounts[existingIndex] = candidate
	} else {
		state.Accounts = append(state.Accounts, candidate)
	}
	if state.ActiveAccountID == "" {
		state.ActiveAccountID = candidate.ID
	}
	if nextID != "" {
		state.RotationCursor = accountPosition(state.Accounts, nextID)
	} else if !hadEnabled {
		state.RotationCursor = accountPosition(state.Accounts, candidate.ID)
	}
}

func (store *FileStore) persistLoginState(state stateFile, backup string, rollback func() error) error {
	committed, err := store.writeState(state)
	if err != nil {
		if committed {
			return err
		}
		if rollbackErr := rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("restore account profile: %w", rollbackErr))
		}
		return err
	}
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove previous account profile backup: %w", err)
		}
	}
	return nil
}
