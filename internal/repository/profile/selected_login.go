package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/christiandoxa/godex/internal/helper/fileutil"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func (store *Store) ApplySelectedLoginMetadata(
	ctx context.Context,
	name string,
	desired profileentity.Profile,
) error {
	if desired.Name != name || desired.Provider.Kind != profileentity.ProviderOpenAI {
		return errors.New("selected login profile metadata is invalid")
	}
	return store.withLock(ctx, func() error {
		release, err := store.acquireMutation(name)
		if err != nil {
			return err
		}
		defer release()
		state, err := store.readState()
		if err != nil {
			return err
		}
		index := profileIndex(state.Profiles, name)
		if index < 0 {
			return fmt.Errorf(profileDoesNotExistFormat, name)
		}
		current := state.Profiles[index]
		if filepath.Clean(current.CodexHome) != filepath.Clean(desired.CodexHome) ||
			current.Provider.Kind != profileentity.ProviderOpenAI {
			return fmt.Errorf("profile %q changed while login was running", name)
		}
		desired.CodexHome = current.CodexHome
		desired.Managed = current.Managed
		if err := profileentity.Validate(desired); err != nil {
			return err
		}
		state.Profiles[index] = desired
		return store.writeState(state)
	})
}

func (store *Store) ApplySelectedLoginFiles(
	ctx context.Context,
	name string,
	files []profilemodel.ExportedSecretFile,
	remove []string,
) error {
	if len(files) == 0 && len(remove) == 0 {
		return nil
	}
	return store.withLock(ctx, func() error {
		release, err := store.acquireMutation(name)
		if err != nil {
			return err
		}
		defer release()
		state, err := store.readState()
		if err != nil {
			return err
		}
		index := profileIndex(state.Profiles, name)
		if index < 0 {
			return fmt.Errorf(profileDoesNotExistFormat, name)
		}
		profile := state.Profiles[index]
		if profile.Provider.Kind != profileentity.ProviderOpenAI {
			return fmt.Errorf("profile %q changed while login was running", name)
		}
		for _, file := range files {
			if file.Path != profileLocalConfigFileName || len(file.Text) > maxProfileLocalConfigBytes {
				return errors.New("selected login profile file is invalid")
			}
			if _, err := fileutil.AtomicWrite(filepath.Join(profile.CodexHome, file.Path), []byte(file.Text)); err != nil {
				return err
			}
		}
		for _, name := range remove {
			if name != profileLocalConfigFileName {
				return errors.New("selected login profile file removal is invalid")
			}
			if err := os.Remove(filepath.Join(profile.CodexHome, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	})
}
