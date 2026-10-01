package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const maxStateBytes = 4 << 20

func (store *Store) readState() (stateFile, error) {
	info, err := os.Lstat(store.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return stateFile{Version: stateVersion}, nil
	}
	if err != nil {
		return stateFile{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxStateBytes {
		return stateFile{}, errors.New("profile state must be a bounded regular file")
	}
	content, err := os.ReadFile(store.statePath())
	if err != nil {
		return stateFile{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var state stateFile
	if err := decoder.Decode(&state); err != nil {
		return stateFile{}, fmt.Errorf("decode profile state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return stateFile{}, errors.New("profile state has trailing data")
	}
	if state.Version != stateVersion {
		return stateFile{}, fmt.Errorf("unsupported profile state version %d", state.Version)
	}
	if err := validateState(state); err != nil {
		return stateFile{}, err
	}
	return state, nil
}

func (store *Store) writeState(state stateFile) error {
	state.Version = stateVersion
	if err := validateState(state); err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	_, err = fileutil.AtomicWrite(store.statePath(), content)
	return err
}

func validateState(state stateFile) error {
	names := make(map[string]bool, len(state.Profiles))
	homes := make(map[string]bool, len(state.Profiles))
	activeFound := state.Active == ""
	for _, current := range state.Profiles {
		if err := profileentity.Validate(current); err != nil {
			return fmt.Errorf("invalid profile state: %w", err)
		}
		if names[current.Name] {
			return fmt.Errorf("duplicate profile name %q", current.Name)
		}
		names[current.Name] = true
		home := current.CodexHome
		if homes[home] {
			return fmt.Errorf("duplicate profile CODEX_HOME %q", home)
		}
		homes[home] = true
		if current.Name == state.Active {
			activeFound = true
		}
	}
	if !activeFound {
		return fmt.Errorf("active profile %q does not exist", state.Active)
	}
	return nil
}
