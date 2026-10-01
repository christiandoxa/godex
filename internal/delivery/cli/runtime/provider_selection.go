package runtime

import (
	"context"
	"errors"
	"fmt"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

const anthropicProviderKind = "anthropic"

func runProfilelessProviderSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	selection runtimemodel.Selection,
	arguments []string,
) error {
	if selection.Provider != anthropicProviderKind {
		return fmt.Errorf("runtime provider shortcut %q is not implemented yet", selection.Provider)
	}
	keys, err := runner.ProviderAPIKeys(selection.Provider, selection.APIKey)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return errors.New("godex run --provider anthropic requires a Claude profile, --api-key, or ANTHROPIC_API_KEY(S)")
	}
	provider := runtimeusecase.AnthropicProvider("anthropic-api-key", selection.BaseURL)
	return runner.RunProviderAPIKeys(ctx, "", provider, keys, arguments)
}

func runProviderSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	arguments []string,
) error {
	if selection.Provider != anthropicProviderKind {
		return fmt.Errorf("runtime provider shortcut %q is not implemented yet", selection.Provider)
	}
	keys, err := runner.ProviderAPIKeys(selection.Provider, selection.APIKey)
	if err != nil {
		return err
	}
	target, found, err := profiles.ResolveProviderLaunch(ctx, selection.Provider, selection.Profile)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return runProviderAPIKeySelection(ctx, runner, profiles, selection, target, found, keys, arguments)
	}
	return runProviderOAuthSelection(ctx, runner, profiles, selection, target, found, arguments)
}

func runProviderAPIKeySelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	target profilemodel.LaunchTarget,
	found bool,
	keys []string,
	arguments []string,
) (runErr error) {
	home := ""
	name := "anthropic-api-key"
	if found {
		home, name = target.CodexHome, target.Name
		if target.Name != "" && target.AccountID == "" {
			release, err := profiles.AcquireLaunch(ctx, target.Name)
			if err != nil {
				return err
			}
			defer func() { runErr = errors.Join(runErr, release()) }()
		}
	}
	provider := runtimeusecase.AnthropicProvider(name, selection.BaseURL)
	if found && target.AccountID != "" {
		return runner.RunProviderAPIKeysAccount(ctx, target.AccountID, provider, keys, arguments)
	}
	return runner.RunProviderAPIKeys(ctx, home, provider, keys, arguments)
}

func runProviderOAuthSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	target profilemodel.LaunchTarget,
	found bool,
	arguments []string,
) error {
	if !found || target.Provider != anthropicProviderKind {
		return errors.New("godex run --provider anthropic requires a Claude profile, --api-key, or ANTHROPIC_API_KEY(S)")
	}
	provider, err := launchRuntimeProvider(target)
	if err != nil {
		return err
	}
	if selection.BaseURL != "" {
		provider.APIURL = selection.BaseURL
	}
	pool, err := profiles.ProviderLaunchPool(ctx, target.Name, anthropicProviderKind, selection.Profile == "")
	if err != nil {
		return err
	}
	return runProviderPool(ctx, runner, profiles, target, provider, pool, arguments, selection.BaseURL)
}
