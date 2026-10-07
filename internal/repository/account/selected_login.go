package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
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

func (store *FileStore) ApplySelectedAPIKey(
	ctx context.Context,
	accountID string,
	authJSON []byte,
	files []profilemodel.ExportedSecretFile,
	remove []string,
) (accountentity.Account, error) {
	if len(authJSON) == 0 || len(authJSON) > 2<<20 {
		return accountentity.Account{}, errors.New("selected API-key authentication is invalid or too large")
	}
	if len(files) > 1 || len(remove) > 1 || len(files) != 0 && len(remove) != 0 {
		return accountentity.Account{}, errors.New("selected API-key local config is invalid")
	}
	for _, file := range files {
		if file.Path != importedAuthLocalConfigFile || len(file.Text) > importedAuthLocalConfigMax {
			return accountentity.Account{}, errors.New("selected API-key local config is invalid")
		}
	}
	for _, name := range remove {
		if name != importedAuthLocalConfigFile {
			return accountentity.Account{}, errors.New("selected API-key local config removal is invalid")
		}
	}

	var result accountentity.Account
	err := store.withLock(ctx, func() error {
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
		home := store.CodexHome(accountID)
		if err := ownerOnlyDirectory(home); err != nil {
			return err
		}

		previousAuth, hadAuth, err := readExistingImportedAuth(filepath.Join(home, authFileName))
		if err != nil {
			return err
		}
		defer clearImportedAuth(previousAuth)
		previousConfig, hadConfig, err := readExistingImportedLocalConfig(filepath.Join(home, importedAuthLocalConfigFile))
		if err != nil {
			return err
		}
		rollbackFiles := func() {
			authPath := filepath.Join(home, authFileName)
			if hadAuth {
				_, _ = fileutil.AtomicWrite(authPath, previousAuth)
			} else {
				_ = os.Remove(authPath)
			}
			if len(files) != 0 || len(remove) != 0 {
				configPath := filepath.Join(home, importedAuthLocalConfigFile)
				if hadConfig {
					_, _ = fileutil.AtomicWrite(configPath, previousConfig)
				} else {
					_ = os.Remove(configPath)
				}
			}
		}

		if _, err := fileutil.AtomicWrite(filepath.Join(home, authFileName), authJSON); err != nil {
			return err
		}
		if len(files) != 0 {
			if _, err := fileutil.AtomicWrite(filepath.Join(home, importedAuthLocalConfigFile), []byte(files[0].Text)); err != nil {
				rollbackFiles()
				return err
			}
		} else if len(remove) != 0 {
			if err := os.Remove(filepath.Join(home, importedAuthLocalConfigFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackFiles()
				return err
			}
		}

		current := state.Accounts[index]
		current.Email = ""
		current.ChatGPTAccountID = ""
		current.UpdatedAt = maxTime(current.UpdatedAt, store.now().UTC())
		if err := accountentity.ValidateStoredAccount(current); err != nil {
			rollbackFiles()
			return err
		}
		state.Accounts[index] = current
		committed, err := store.writeState(state)
		if err != nil {
			if !committed {
				rollbackFiles()
			}
			return err
		}
		result = current
		return nil
	})
	return result, err
}

func (store *FileStore) SelectedLoginActionCommitted(
	ctx context.Context,
	action profilemodel.ImportLifecycleAction,
) (bool, error) {
	if action.AccountID == "" || action.Name == "" {
		return false, errors.New("invalid selected login commit check")
	}
	var committed bool
	err := store.withLock(ctx, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		index, err := resolveIndex(state.Accounts, action.AccountID)
		if err != nil || state.Accounts[index].ID != action.AccountID {
			return nil
		}
		account := state.Accounts[index]
		if account.Name != action.Name || store.CodexHome(account.ID) != action.After.CodexHome || account.Email != action.After.Email {
			return nil
		}
		if action.IdentityCleared && (strings.TrimSpace(account.Email) != "" || strings.TrimSpace(account.ChatGPTAccountID) != "") {
			return nil
		}
		for _, file := range action.Files {
			matches, err := selectedLoginFileMatches(store.CodexHome(account.ID), file)
			if err != nil || !matches {
				return err
			}
		}
		committed = true
		return nil
	})
	return committed, err
}

func selectedLoginFileMatches(home string, file profilemodel.ImportLifecycleFile) (bool, error) {
	if file.Path != authFileName && file.Path != importedAuthLocalConfigFile {
		return false, errors.New("invalid selected login lifecycle file")
	}
	path := filepath.Join(home, file.Path)
	info, err := os.Lstat(path)
	if file.Missing {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	}
	if len(file.SHA256) != sha256.Size*2 {
		return false, errors.New("invalid selected login lifecycle digest")
	}
	if _, err := hex.DecodeString(file.SHA256); err != nil {
		return false, errors.New("invalid selected login lifecycle digest")
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	limit := int64(2 << 20)
	if file.Path == importedAuthLocalConfigFile {
		limit = importedAuthLocalConfigMax
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > limit {
		return false, errors.New("selected login lifecycle file is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return false, errors.New("selected login lifecycle file is not private")
	}
	content, err := os.ReadFile(path)
	if err != nil || int64(len(content)) > limit {
		return false, errors.New("selected login lifecycle file is unavailable")
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]) == file.SHA256, nil
}
