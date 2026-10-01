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
		return runProviderAPIKeySelection(ctx, runner, providerAPIKeyRequest{
			profiles: profiles, selection: selection, target: target, found: found,
			keys: keys, arguments: arguments,
		})
	}
	return runProviderOAuthSelection(ctx, runner, profiles, selection, target, found, arguments)
}

type providerAPIKeyRequest struct {
	profiles  launchProfiles
	selection runtimemodel.Selection
	target    profilemodel.LaunchTarget
	found     bool
	keys      []string
	arguments []string
}

func runProviderAPIKeySelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	request providerAPIKeyRequest,
) (runErr error) {
	home := ""
	name := "anthropic-api-key"
	if request.found {
		home, name = request.target.CodexHome, request.target.Name
		if request.target.Name != "" && request.target.AccountID == "" {
			release, err := request.profiles.AcquireLaunch(ctx, request.target.Name)
			if err != nil {
				return err
			}
			defer func() { runErr = errors.Join(runErr, release()) }()
		}
	}
	provider := runtimeusecase.AnthropicProvider(name, request.selection.BaseURL)
	if request.found && request.target.AccountID != "" {
		return runner.RunProviderAPIKeysAccount(
			ctx, request.target.AccountID, provider, request.keys, request.arguments,
		)
	}
	return runner.RunProviderAPIKeys(ctx, home, provider, request.keys, request.arguments)
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
	return runProviderPool(ctx, runner, providerPoolRequest{
		profiles: profiles, selected: target, provider: provider, pool: pool,
		arguments: arguments, apiURLOverride: selection.BaseURL,
	})
}
