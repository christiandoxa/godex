package runtime

import (
	"context"
	"errors"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

type superLocalSessionLauncher struct {
	runner   *runtimeusecase.Runner
	profiles launchProfiles
	profile  string
	config   runtimeusecase.LocalProviderConfig
	options  runtimeusecase.RuntimeLaunchOptions
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

func launchSuperLocal(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	profiles launchProfiles,
	options superOptions,
) error {
	config := superLocalProviderConfig(options)
	launchOptions := superLocalRuntimeLaunchOptions(options)
	launchArguments := superPreparedCodexArgs(options)
	if index, args := sessionArgument(options.codexArgs); index >= 0 {
		if sessions == nil {
			return errors.New("session support is not configured")
		}
		prefix, selector := splitThreadSelector(args[index])
		return sessions.ResumeArgumentsWithLauncher(ctx, sessionmodel.Launch{
			ProfileSelector: options.profile, SessionSelector: selector, IDIndex: index, IDPrefix: prefix, Arguments: args, Local: true,
		}, superLocalSessionLauncher{runner: runner, profiles: profiles, profile: options.profile, config: config, options: launchOptions})
	}
	if options.profile != "" {
		if profiles == nil {
			return errors.New("--profile requires profile-aware Super dispatch")
		}
		target, err := profiles.ResolveLaunch(ctx, options.profile)
		if err != nil {
			return err
		}
		return launchSuperLocalTarget(ctx, runner, profiles, target, config, launchArguments, launchOptions)
	}
	if profiles != nil {
		target, active, err := profiles.ActiveLaunch(ctx)
		if err != nil {
			return err
		}
		if active {
			if target.AccountID == "" && strings.TrimSpace(target.Name) == "" {
				return runner.RunLocalRewriteHomeWithOptions(ctx, target.CodexHome, config, launchArguments, launchOptions)
			}
			return launchSuperLocalTarget(ctx, runner, profiles, target, config, launchArguments, launchOptions)
		}
	}
	return runner.RunLocalRewriteWithOptions(ctx, "", config, launchArguments, launchOptions)
}

func (launcher superLocalSessionLauncher) RunSessionReport(ctx context.Context, report sessionmodel.Report, args []string, _ bool) error {
	if launcher.profile == "" {
		return launcher.RunSession(ctx, report.AccountID, report.UpstreamAccountID, args)
	}
	if launcher.profiles == nil {
		return errors.New("--profile requires profile-aware Super dispatch")
	}
	target, err := launcher.profiles.ResolveLaunch(ctx, launcher.profile)
	if err != nil {
		return err
	}
	return launchSuperLocalTarget(ctx, launcher.runner, launcher.profiles, target, launcher.config, ensureSuperFullAccess(args), launcher.options)
}

func launchSuperLocalTarget(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	target profilemodel.LaunchTarget,
	config runtimeusecase.LocalProviderConfig,
	args []string,
	options runtimeusecase.RuntimeLaunchOptions,
) (runErr error) {
	if target.AccountID != "" {
		return runner.RunLocalRewriteWithOptions(ctx, target.AccountID, config, args, options)
	}
	if strings.TrimSpace(target.Name) == "" {
		return errors.New("profile launch metadata is incomplete")
	}
	release, err := profiles.AcquireLaunch(ctx, target.Name)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	return runner.RunLocalRewriteHomeWithOptions(ctx, target.CodexHome, config, args, options)
}
