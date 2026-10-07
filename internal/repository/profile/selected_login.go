package profile

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
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
