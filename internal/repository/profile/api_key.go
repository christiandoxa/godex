package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

func (store *Store) LoginOpenAIAPIKey(
	ctx context.Context,
	profile profileentity.Profile,
	authJSON []byte,
	baseURL *string,
	baseURLSpecified bool,
	activate bool,
) (profileentity.Profile, bool, error) {
	request := apiKeyLoginRequest{
		Profile: profile, AuthJSON: authJSON, BaseURL: baseURL,
		BaseURLSpecified: baseURLSpecified, Activate: activate,
	}
	if err := validateAPIKeyLoginRequest(request); err != nil {
		return profileentity.Profile{}, false, err
	}
	var result profileentity.Profile
	var created bool
	err := store.withLock(ctx, func() error {
		var lockedErr error
		result, created, lockedErr = store.loginOpenAIAPIKeyLocked(request)
		return lockedErr
	})
	return result, created, err
}

func validateAPIKeyLoginRequest(request apiKeyLoginRequest) error {
	if err := profileentity.Validate(request.Profile); err != nil {
		return err
	}
	if request.Profile.Provider.Kind != profileentity.ProviderOpenAI {
		return errors.New("API-key login requires an OpenAI profile")
	}
	if err := validateAuthJSONBytes(request.AuthJSON); err != nil {
		return err
	}
	if request.BaseURLSpecified && request.BaseURL != nil {
		_, err := validateOpenAICompatibleBaseURL(*request.BaseURL)
		return err
	}
	return nil
}

func (store *Store) loginOpenAIAPIKeyLocked(request apiKeyLoginRequest) (profileentity.Profile, bool, error) {
	state, err := store.readState()
	if err != nil {
		return profileentity.Profile{}, false, err
	}
	index := profileIndex(state.Profiles, request.Profile.Name)
	if index >= 0 {
		current := state.Profiles[index]
		if current.Provider.Kind != profileentity.ProviderOpenAI {
			return profileentity.Profile{}, false, fmt.Errorf("profile %q does not support Codex API-key login", request.Profile.Name)
		}
		if err := store.updateAPIKeyProfile(state, index, request); err != nil {
			return profileentity.Profile{}, false, err
		}
		return request.Profile, false, nil
	}
	if err := store.createAPIKeyProfile(state, request); err != nil {
		return profileentity.Profile{}, false, err
	}
	return request.Profile, true, nil
}

type apiKeyLoginRequest struct {
	Profile          profileentity.Profile
	AuthJSON         []byte
	BaseURL          *string
	BaseURLSpecified bool
	Activate         bool
}

func (store *Store) createAPIKeyProfile(state stateFile, request apiKeyLoginRequest) error {
	if !request.Profile.Managed {
		return errors.New("API-key login must use managed profile storage")
	}
	if err := validateNewProfile(state.Profiles, request.Profile); err != nil {
		return err
	}
	staged, err := store.stageImportedAuthHome(request.AuthJSON)
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	if request.BaseURLSpecified {
		if err := writeOpenAICompatibleBaseURL(staged, request.BaseURL); err != nil {
			return err
		}
	}
	if err := os.Rename(staged, request.Profile.CodexHome); err != nil {
		return fmt.Errorf("promote API-key profile: %w", err)
	}
	state.Profiles = append(state.Profiles, request.Profile)
	if request.Activate || state.Active == "" {
		state.Active = request.Profile.Name
	}
	if err := store.writeState(state); err != nil {
		_ = os.RemoveAll(request.Profile.CodexHome)
		return err
	}
	return nil
}

func (store *Store) updateAPIKeyProfile(state stateFile, index int, request apiKeyLoginRequest) error {
	current := state.Profiles[index]
	release, err := store.acquireMutation(current.Name)
	if err != nil {
		return err
	}
	defer release()
	previousAuth, err := store.ReadAuthJSON(current.CodexHome)
	if err != nil {
		return err
	}
	defer clearBytes(previousAuth)
	previousConfig, configFound, err := profileLocalConfigBackup(current.CodexHome)
	if err != nil {
		return err
	}
	defer clearBytes(previousConfig)
	if _, err := fileutil.AtomicWrite(filepath.Join(current.CodexHome, profileAuthFileName), request.AuthJSON); err != nil {
		return err
	}
	rollbackFiles := func() {
		_, _ = fileutil.AtomicWrite(filepath.Join(current.CodexHome, profileAuthFileName), previousAuth)
		_ = restoreProfileLocalConfig(current.CodexHome, previousConfig, configFound)
	}
	if request.BaseURLSpecified {
		if err := writeOpenAICompatibleBaseURL(current.CodexHome, request.BaseURL); err != nil {
			rollbackFiles()
			return err
		}
	}
	updated := request.Profile
	updated.CodexHome = current.CodexHome
	updated.Managed = current.Managed
	state.Profiles[index] = updated
	if request.Activate {
		state.Active = updated.Name
	}
	if err := store.writeState(state); err != nil {
		rollbackFiles()
		return err
	}
	return nil
}
