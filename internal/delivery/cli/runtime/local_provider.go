package runtime

import (
	"context"
	"errors"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func runLocalProviderSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	arguments []string,
) error {
	config := localProviderConfig(selection)
	switch {
	case selection.Profile != "":
		return runLocalProviderProfile(ctx, runner, profiles, selection.Profile, config, arguments)
	case selection.Account != "":
		return runner.RunLocalProvider(ctx, selection.Account, config, arguments)
	default:
		return runDefaultLocalProvider(ctx, runner, profiles, config, arguments)
	}
}

func runLocalProviderProfile(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	name string,
	config runtimeusecase.LocalProviderConfig,
	arguments []string,
) (runErr error) {
	if profiles == nil {
		return errors.New("profile support is not configured")
	}
	target, err := profiles.ResolveLaunch(ctx, name)
	if err != nil {
		return err
	}
	if target.AccountID != "" {
		return runner.RunLocalProvider(ctx, target.AccountID, config, arguments)
	}
	release, err := profiles.AcquireLaunch(ctx, target.Name)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	return runner.RunLocalProviderHome(ctx, target.CodexHome, config, arguments)
}

func runDefaultLocalProvider(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	config runtimeusecase.LocalProviderConfig,
	arguments []string,
) (runErr error) {
	if profiles == nil {
		return runner.RunLocalProvider(ctx, "", config, arguments)
	}
	target, active, err := profiles.ActiveLaunch(ctx)
	if err != nil {
		return err
	}
	if !active {
		return runner.RunLocalProvider(ctx, "", config, arguments)
	}
	release, err := profiles.AcquireLaunch(ctx, target.Name)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	return runner.RunLocalProviderHome(ctx, target.CodexHome, config, arguments)
}

func localProviderConfig(selection runtimemodel.Selection) runtimeusecase.LocalProviderConfig {
	return runtimeusecase.LocalProviderConfig{
		URL: selection.URL, Model: selection.Model,
		ContextWindow:         selection.ContextWindow,
		AutoCompactTokenLimit: selection.AutoCompactTokenLimit,
	}
}
