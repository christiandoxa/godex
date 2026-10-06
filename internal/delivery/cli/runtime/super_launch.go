package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

func superRuntimeLaunchOptions(options superOptions) runtimeusecase.RuntimeLaunchOptions {
	autoRedeem := options.autoRedeem
	allowRotate := !options.noAutoRotate
	launch := runtimeusecase.RuntimeLaunchOptions{
		SmartContextEnabled: true,
		SkipQuotaPreflight:  options.skipQuota,
		AutoRedeem:          &autoRedeem,
		AllowAutoRotate:     &allowRotate,
		SuperOverlay:        true,
		UpstreamNoProxy:     options.noProxy,
		PresidioEnabled:     superPresidioEnabled(options),
		PresidioRequired:    superPresidioRequired(options),
	}
	prepares := make([]func(string) error, 0, 2)
	if options.toolResolutionDone {
		tools := make([]runtimeusecase.SuperOptionalTool, 0, len(options.resolvedTools))
		for _, status := range options.resolvedTools {
			if status.service || !status.resolved {
				continue
			}
			tools = append(tools, runtimeusecase.SuperOptionalTool{
				Name: status.name, Path: status.path, Required: status.required,
			})
		}
		presidio := superPresidioEnabled(options)
		prepares = append(prepares, func(home string) error {
			return runtimeusecase.PrepareSuperToolsOverlay(home, tools, presidio)
		})
	}
	if options.subAgent.enabled {
		config := runtimeusecase.SuperSubAgentConfig{
			Provider:             options.subAgent.provider,
			Model:                options.subAgent.model,
			Effort:               options.subAgent.effort,
			LocalURL:             options.subAgent.url,
			MaxConcurrency:       options.subAgent.maxConcurrency,
			MaxConcurrencySource: options.subAgent.maxConcurrencySource,
			PresidioEnabled:      superPresidioEnabled(options),
			RequiredTools:        append([]string(nil), options.requiredTools...),
		}
		prepares = append(prepares, func(home string) error {
			return runtimeusecase.PrepareSuperSubAgentOverlay(home, config)
		})
	}
	if len(prepares) > 0 {
		launch.OverlayPrepare = func(home string) error {
			for _, prepare := range prepares {
				if err := prepare(home); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return launch
}

type superSessionLauncher struct {
	runner  *runtimeusecase.Runner
	options runtimeusecase.RuntimeLaunchOptions
}

func (launcher superSessionLauncher) Run(ctx context.Context, selector string, args []string) error {
	return launcher.runner.RunWithOptions(ctx, selector, ensureSuperFullAccess(args), launcher.options)
}

func (launcher superSessionLauncher) RunLocal(ctx context.Context, selector string, args []string) error {
	return launcher.runner.RunLocal(ctx, selector, args)
}

func (launcher superSessionLauncher) RunSession(ctx context.Context, homeID, ownerID string, args []string) error {
	return launcher.runner.RunSessionWithOptions(
		ctx, homeID, ownerID, ensureSuperFullAccess(args), launcher.options,
	)
}

type superLocalSessionLauncher struct {
	runner  *runtimeusecase.Runner
	config  runtimeusecase.LocalProviderConfig
	options runtimeusecase.RuntimeLaunchOptions
}

func (launcher superLocalSessionLauncher) Run(ctx context.Context, selector string, args []string) error {
	return launcher.runner.RunLocalRewriteWithOptions(ctx, selector, launcher.config, ensureSuperFullAccess(args), launcher.options)
}

func (launcher superLocalSessionLauncher) RunLocal(ctx context.Context, selector string, args []string) error {
	return launcher.runner.RunLocalRewriteAccountWithOptions(ctx, selector, launcher.config, ensureSuperFullAccess(args), launcher.options)
}

func (launcher superLocalSessionLauncher) RunSession(ctx context.Context, homeID, _ string, args []string) error {
	return launcher.runner.RunLocalRewriteAccountWithOptions(ctx, homeID, launcher.config, ensureSuperFullAccess(args), launcher.options)
}

func superLocalRuntimeLaunchOptions(options superOptions) runtimeusecase.RuntimeLaunchOptions {
	launch := superRuntimeLaunchOptions(options)
	prior := launch.OverlayPrepare
	launch.OverlayPrepare = func(home string) error {
		if err := runtimeusecase.PrepareLocalRewriteOverlayAuth(home); err != nil {
			return err
		}
		if prior != nil {
			return prior(home)
		}
		return nil
	}
	return launch
}

func superLocalProviderConfig(options superOptions) runtimeusecase.LocalProviderConfig {
	return runtimeusecase.LocalProviderConfig{
		URL: options.localURL, Model: options.model,
		ContextWindow: options.contextWindow, AutoCompactTokenLimit: options.autoCompact,
	}
}

func launchSuperProfiles(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	profiles launchProfiles,
	options superOptions,
) error {
	if runner == nil {
		return errors.New("runtime support is not configured")
	}
	if options.localURL != "" {
		return launchSuperLocal(ctx, runner, sessions, profiles, options)
	}
	if options.cli == "agy" {
		return runner.RunAntigravity(ctx, options.model, options.codexArgs)
	}

	launchOptions := superRuntimeLaunchOptions(options)
	launchArguments := superPreparedCodexArgs(options)
	if options.provider == "" {
		if index, args := sessionArgument(options.codexArgs); index >= 0 {
			if sessions == nil {
				return errors.New("session support is not configured")
			}
			prefix, selector := splitThreadSelector(args[index])
			return sessions.ResumeArgumentsWithLauncher(ctx, sessionmodel.Launch{
				SessionSelector: selector,
				IDIndex:         index,
				IDPrefix:        prefix,
				Arguments:       args,
				Local:           false,
			}, superSessionLauncher{runner: runner, options: launchOptions})
		}
	}

	if options.provider != "" {
		if profiles == nil {
			return launchSuperProfilelessProvider(ctx, runner, options, launchOptions)
		}
		return launchSuperProviderSelection(ctx, runner, profiles, options, launchOptions)
	}
	if options.profile != "" {
		if profiles == nil {
			return errors.New("--profile requires profile-aware Super dispatch")
		}
		target, err := profiles.ResolveLaunch(ctx, options.profile)
		if err != nil {
			return err
		}
		return launchSuperTarget(ctx, runner, profiles, target, options, false)
	}
	if profiles != nil {
		target, active, err := profiles.ActiveLaunch(ctx)
		if err != nil {
			return err
		}
		if active {
			return launchSuperTarget(ctx, runner, profiles, target, options, true)
		}
	}
	return runner.RunWithOptions(ctx, "", launchArguments, launchOptions)
}

func launchSuperLocal(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	profiles launchProfiles,
	options superOptions,
) (runErr error) {
	config := superLocalProviderConfig(options)
	launchOptions := superLocalRuntimeLaunchOptions(options)
	launchArguments := superPreparedCodexArgs(options)
	if index, args := sessionArgument(options.codexArgs); index >= 0 {
		if sessions == nil {
			return errors.New("session support is not configured")
		}
		prefix, selector := splitThreadSelector(args[index])
		return sessions.ResumeArgumentsWithLauncher(ctx, sessionmodel.Launch{
			SessionSelector: selector, IDIndex: index, IDPrefix: prefix, Arguments: args, Local: true,
		}, superLocalSessionLauncher{runner: runner, config: config, options: launchOptions})
	}
	if options.profile != "" {
		if profiles == nil {
			return errors.New("--profile requires profile-aware Super dispatch")
		}
		target, err := profiles.ResolveLaunch(ctx, options.profile)
		if err != nil {
			return err
		}
		if target.AccountID != "" {
			return runner.RunLocalRewriteWithOptions(ctx, target.AccountID, config, launchArguments, launchOptions)
		}
		if strings.TrimSpace(target.Name) == "" {
			return errors.New("profile launch metadata is incomplete")
		}
		release, err := profiles.AcquireLaunch(ctx, target.Name)
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, release()) }()
		return runner.RunLocalRewriteHomeWithOptions(ctx, target.CodexHome, config, launchArguments, launchOptions)
	}
	if profiles != nil {
		target, active, err := profiles.ActiveLaunch(ctx)
		if err != nil {
			return err
		}
		if active {
			if target.AccountID != "" {
				return runner.RunLocalRewriteWithOptions(ctx, target.AccountID, config, launchArguments, launchOptions)
			}
			if strings.TrimSpace(target.Name) != "" {
				release, err := profiles.AcquireLaunch(ctx, target.Name)
				if err != nil {
					return err
				}
				defer func() { runErr = errors.Join(runErr, release()) }()
			}
			return runner.RunLocalRewriteHomeWithOptions(ctx, target.CodexHome, config, launchArguments, launchOptions)
		}
	}
	return runner.RunLocalRewriteWithOptions(ctx, "", config, launchArguments, launchOptions)
}

func launchSuperTarget(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	target profilemodel.LaunchTarget,
	options superOptions,
	allowProviderPool bool,
) (runErr error) {
	launchOptions := superRuntimeLaunchOptions(options)
	launchArguments := superPreparedCodexArgs(options)
	if target.AccountID != "" {
		return runner.RunWithOptions(ctx, target.AccountID, launchArguments, launchOptions)
	}
	if strings.TrimSpace(target.Name) == "" {
		return errors.New("profile launch metadata is incomplete")
	}
	provider, err := launchRuntimeProvider(target)
	if err != nil {
		return err
	}
	if provider.Kind == "copilot" || provider.Kind == "anthropic" || provider.Kind == "kiro" {
		pool, err := profiles.ProviderLaunchPool(
			ctx, target.Name, target.Provider, allowProviderPool && !options.noAutoRotate,
		)
		if err != nil {
			return err
		}
		return launchSuperProviderPool(ctx, runner, superProviderPoolRequest{
			profiles: profiles, selected: target, provider: provider, pool: pool,
			arguments: launchArguments, options: launchOptions,
		})
	}
	release, err := profiles.AcquireLaunch(ctx, target.Name)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	if target.Provider == "openai" {
		baseURL, compatible, err := profiles.OpenAICompatibleBaseURL(ctx, target.Name)
		if err != nil {
			return err
		}
		if compatible {
			return fmt.Errorf(
				"Godex Super profile %q uses OpenAI-compatible base URL %s and requires local-rewrite proxy support",
				target.Name, baseURL,
			)
		}
	}
	return runner.RunProfileWithOptions(ctx, target.CodexHome, launchArguments, launchOptions)
}

func launchSuperProfilelessProvider(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	options superOptions,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) error {
	if options.provider == kiroProviderKind {
		return providerCredentialRequired(kiroProviderKind)
	}
	keys, err := runner.ProviderAPIKeys(options.provider, options.apiKey)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return providerCredentialRequired(options.provider)
	}
	provider, err := externalAPIKeyProvider(options.provider, options.provider+"-api-key", options.baseURL)
	if err != nil {
		return err
	}
	applyProviderSelectionModel(&provider, options.model)
	if err := runtimeusecase.ApplyProviderSelectionLimits(
		&provider, options.contextWindow, options.autoCompact,
	); err != nil {
		return err
	}
	return runner.RunProviderAPIKeysWithOptions(
		ctx, "", provider, keys, superPreparedCodexArgs(options), launchOptions,
	)
}

func launchSuperProviderSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	options superOptions,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) error {
	target, found, err := profiles.ResolveProviderLaunch(ctx, options.provider, options.profile)
	if err != nil {
		return err
	}
	if options.provider == kiroProviderKind {
		if !found || target.Provider != kiroProviderKind {
			return providerCredentialRequired(kiroProviderKind)
		}
		release, err := profiles.AcquireLaunch(ctx, target.Name)
		if err != nil {
			return err
		}
		defer release()
		provider := runtimeusecase.KiroProvider(target.Name)
		applyProviderSelectionModel(&provider, options.model)
		if err := runtimeusecase.ApplyProviderSelectionLimits(
			&provider, options.contextWindow, options.autoCompact,
		); err != nil {
			return err
		}
		return runner.RunProviderProfileWithOptions(
			ctx, target.CodexHome, provider, superPreparedCodexArgs(options), launchOptions,
		)
	}

	keys, err := runner.ProviderAPIKeys(options.provider, options.apiKey)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return launchSuperProviderAPIKeys(
			ctx, runner, profiles, options, target, found, keys, launchOptions,
		)
	}
	if options.provider == anthropicProviderKind {
		if !found || target.Provider != anthropicProviderKind {
			return providerCredentialRequired(anthropicProviderKind)
		}
		provider, err := launchRuntimeProvider(target)
		if err != nil {
			return err
		}
		if options.baseURL != "" {
			provider.APIURL = options.baseURL
		}
		applyProviderSelectionModel(&provider, options.model)
		if err := runtimeusecase.ApplyProviderSelectionLimits(
			&provider, options.contextWindow, options.autoCompact,
		); err != nil {
			return err
		}
		pool, err := profiles.ProviderLaunchPool(
			ctx, target.Name, anthropicProviderKind, options.profile == "" && !options.noAutoRotate,
		)
		if err != nil {
			return err
		}
		return launchSuperProviderPool(ctx, runner, superProviderPoolRequest{
			profiles: profiles, selected: target, provider: provider, pool: pool,
			arguments: superPreparedCodexArgs(options), apiURLOverride: options.baseURL, options: launchOptions,
		})
	}
	return providerCredentialRequired(options.provider)
}

func launchSuperProviderAPIKeys(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	options superOptions,
	target profilemodel.LaunchTarget,
	found bool,
	keys []string,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) (runErr error) {
	home := ""
	name := options.provider + "-api-key"
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
	provider, err := externalAPIKeyProvider(options.provider, name, options.baseURL)
	if err != nil {
		return err
	}
	applyProviderSelectionModel(&provider, options.model)
	if err := runtimeusecase.ApplyProviderSelectionLimits(
		&provider, options.contextWindow, options.autoCompact,
	); err != nil {
		return err
	}
	if found && target.AccountID != "" {
		return runner.RunProviderAPIKeysAccountWithOptions(
			ctx, target.AccountID, provider, keys, superPreparedCodexArgs(options), launchOptions,
		)
	}
	return runner.RunProviderAPIKeysWithOptions(
		ctx, home, provider, keys, superPreparedCodexArgs(options), launchOptions,
	)
}

type superProviderPoolRequest struct {
	profiles       launchProfiles
	selected       profilemodel.LaunchTarget
	provider       proxymodel.Provider
	pool           []profilemodel.LaunchTarget
	arguments      []string
	apiURLOverride string
	options        runtimeusecase.RuntimeLaunchOptions
}

func launchSuperProviderPool(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	request superProviderPoolRequest,
) (runErr error) {
	names := make([]string, 0, len(request.pool))
	runtimeProfiles := make([]proxymodel.ProviderProfile, 0, len(request.pool))
	for _, target := range request.pool {
		currentProvider, err := launchRuntimeProvider(target)
		if err != nil {
			return err
		}
		if currentProvider.Kind != request.provider.Kind {
			continue
		}
		if request.apiURLOverride != "" {
			currentProvider.APIURL = request.apiURLOverride
		}
		names = append(names, target.Name)
		runtimeProfiles = append(runtimeProfiles, proxymodel.ProviderProfile{
			Name: target.Name, Home: target.CodexHome, Provider: currentProvider, Enabled: true,
		})
	}
	if len(runtimeProfiles) == 0 {
		return errors.New("runtime provider pool is empty")
	}
	release, err := request.profiles.AcquireLaunchPool(ctx, names)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	return runner.RunProviderProfilesWithOptions(
		ctx, request.selected.CodexHome, request.provider, runtimeProfiles,
		request.arguments, request.options,
	)
}
