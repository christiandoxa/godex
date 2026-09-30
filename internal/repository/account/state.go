package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"io"
	"os"
	"path/filepath"
	"strings"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

func (store *FileStore) readState() (stateFile, error) {
	if err := store.checkRoot(); err != nil {
		return stateFile{}, err
	}
	path := store.statePath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return stateFile{Version: stateVersion}, nil
	}
	if err != nil {
		return stateFile{}, fmt.Errorf("read Godex state: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return stateFile{}, errors.New("Godex state must not be a symbolic link")
	}
	if !info.Mode().IsRegular() {
		return stateFile{}, errors.New("Godex state is not a regular file")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return stateFile{}, fmt.Errorf("secure Godex state: %w", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return stateFile{}, fmt.Errorf("read Godex state: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var state stateFile
	if err := decoder.Decode(&state); err != nil {
		return stateFile{}, fmt.Errorf("decode Godex state: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return stateFile{}, err
	}
	if state.Version != stateVersion {
		return stateFile{}, fmt.Errorf("unsupported Godex state version %d; expected %d", state.Version, stateVersion)
	}
	if err := validateState(state); err != nil {
		return stateFile{}, err
	}
	return state, nil
}

func (store *FileStore) writeState(state stateFile) (bool, error) {
	state.Version = stateVersion
	if err := validateState(state); err != nil {
		return false, err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode Godex state: %w", err)
	}
	content = append(content, '\n')

	temporary, err := os.CreateTemp(store.root, ".state-*")
	if err != nil {
		return false, fmt.Errorf("create temporary state: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return false, fmt.Errorf("secure temporary state: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		return false, fmt.Errorf("write temporary state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return false, fmt.Errorf("sync temporary state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close temporary state: %w", err)
	}
	if err := fileutil.Replace(temporaryPath, store.statePath()); err != nil {
		return false, fmt.Errorf("replace Godex state: %w", err)
	}
	if err := fileutil.SyncDirectory(filepath.Dir(store.statePath())); err != nil {
		return true, fmt.Errorf("sync Godex state directory: %w", err)
	}
	return true, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing Godex state: %w", err)
	}
	return errors.New("Godex state contains multiple JSON values")
}

func validateState(state stateFile) error {
	ids := make(map[string]struct{}, len(state.Accounts))
	names := make(map[string]struct{}, len(state.Accounts))
	activeFound := state.ActiveAccountID == ""

	for index, account := range state.Accounts {
		if err := entity.ValidateAccount(account); err != nil {
			return fmt.Errorf("Godex state contains invalid account metadata: %w", err)
		}
		if _, exists := ids[account.ID]; exists {
			return fmt.Errorf("Godex state contains duplicate account ID %q", account.ID)
		}
		ids[account.ID] = struct{}{}

		name := strings.ToLower(account.Name)
		if _, exists := names[name]; exists {
			return fmt.Errorf("Godex state contains duplicate account name %q", account.Name)
		}
		names[name] = struct{}{}
		// ponytail: O(n²) identity scan; state is a small local account list.
		for _, previous := range state.Accounts[:index] {
			if previous.SameIdentity(entity.Identity{
				Email:            account.Email,
				ChatGPTAccountID: account.ChatGPTAccountID,
			}) {
				return fmt.Errorf("Godex state contains duplicate account identity %q", account.ID)
			}
		}
		if account.ID == state.ActiveAccountID {
			activeFound = true
		}
	}
	if !activeFound {
		return fmt.Errorf("active account %q does not exist", state.ActiveAccountID)
	}
	return nil
}
