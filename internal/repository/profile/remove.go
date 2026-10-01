package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
)

func (store *Store) Remove(ctx context.Context, name string, deleteHome bool) (profileentity.Profile, error) {
	var removed profileentity.Profile
	err := store.withLock(ctx, func() error {
		release, err := store.acquireMutation(name)
		if err != nil {
			return err
		}
		defer release()
		removed, err = store.removeLocked(name, deleteHome)
		return err
	})
	return removed, err
}

func (store *Store) removeLocked(name string, deleteHome bool) (profileentity.Profile, error) {
	state, err := store.readState()
	if err != nil {
		return profileentity.Profile{}, err
	}
	index := profileIndex(state.Profiles, name)
	if index < 0 {
		return profileentity.Profile{}, fmt.Errorf(profileDoesNotExistFormat, name)
	}
	removed := state.Profiles[index]
	if deleteHome && !removed.Managed {
		return profileentity.Profile{}, fmt.Errorf("refusing to delete external path %s", removed.CodexHome)
	}
	quarantine, err := store.quarantineHome(removed, deleteHome)
	if err != nil {
		return profileentity.Profile{}, err
	}
	state.Profiles = append(state.Profiles[:index], state.Profiles[index+1:]...)
	repairActive(&state, removed.Name)
	if err := store.writeState(state); err != nil {
		restoreQuarantinedHome(quarantine, removed.CodexHome)
		return profileentity.Profile{}, err
	}
	if quarantine != "" {
		if err := os.RemoveAll(quarantine); err != nil {
			return profileentity.Profile{}, err
		}
	}
	return removed, nil
}

func restoreQuarantinedHome(quarantine, destination string) {
	if quarantine != "" {
		_ = os.Rename(quarantine, destination)
	}
}

func (store *Store) quarantineHome(value profileentity.Profile, deleteHome bool) (string, error) {
	if !deleteHome {
		return "", nil
	}
	managedRoot := filepath.Clean(store.profilesRoot())
	relative, err := filepath.Rel(managedRoot, filepath.Clean(value.CodexHome))
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
		return "", errors.New("refusing to delete managed profile outside profiles root")
	}
	if len(relative) >= 3 && relative[:3] == ".."+string(os.PathSeparator) {
		return "", errors.New("refusing to delete managed profile outside profiles root")
	}
	if _, err := os.Lstat(value.CodexHome); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	quarantine := filepath.Join(managedRoot, fmt.Sprintf(".remove-%s-%d", value.Name, time.Now().UnixNano()))
	if err := os.Rename(value.CodexHome, quarantine); err != nil {
		return "", err
	}
	return quarantine, nil
}

func profileIndex(values []profileentity.Profile, name string) int {
	for index, current := range values {
		if current.Name == name {
			return index
		}
	}
	return -1
}

func repairActive(state *stateFile, removed string) {
	if state.Active != removed {
		return
	}
	state.Active = ""
	if len(state.Profiles) == 0 {
		return
	}
	names := make([]string, 0, len(state.Profiles))
	for _, current := range state.Profiles {
		names = append(names, current.Name)
	}
	sort.Strings(names)
	state.Active = names[0]
}
