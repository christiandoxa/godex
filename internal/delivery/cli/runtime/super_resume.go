package runtime

import (
	"context"
	"errors"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

func resolveSuperResumeOptions(
	ctx context.Context,
	sessions *sessionusecase.Catalog,
	options superOptions,
) (superOptions, error) {
	index, args := sessionArgument(options.codexArgs)
	if index < 0 {
		return options, nil
	}
	if sessions == nil {
		return superOptions{}, errors.New("session support is not configured")
	}
	prefix, selector := splitThreadSelector(args[index])
	report, resolved, err := sessions.ResolveArguments(ctx, sessionmodel.Launch{
		ProfileSelector: options.profile,
		SessionSelector: selector,
		IDIndex:         index,
		IDPrefix:        prefix,
		Arguments:       args,
		Local:           options.localURL != "",
	})
	if err != nil {
		return superOptions{}, err
	}
	if options.model != "" {
		report.LastModel = ""
	}
	options.codexArgs = restoreResumeSessionSettings(resolved, report)
	if report.ProfileFallback || options.provider != "" || options.localURL != "" {
		return options, nil
	}
	if _, explicit := runDryConfigOverride(options.codexArgs, "model_provider"); explicit {
		return options, nil
	}
	plan, err := resumeProviderIdentity(report.ModelProvider)
	if err != nil {
		return superOptions{}, err
	}
	if plan.kind != "" {
		options.provider = plan.kind
		options.skipQuota = true
	}
	return options, nil
}

type superSessionLauncher struct {
	runner   *runtimeusecase.Runner
	profiles launchProfiles
	options  superOptions
}

func (launcher superSessionLauncher) Run(ctx context.Context, selector string, args []string) error {
	return launcher.runner.RunWithOptions(ctx, selector, ensureSuperFullAccess(args), superRuntimeLaunchOptions(launcher.options))
}

func (launcher superSessionLauncher) RunLocal(ctx context.Context, selector string, args []string) error {
	return launcher.runner.RunLocal(ctx, selector, args)
}

func (launcher superSessionLauncher) RunSession(ctx context.Context, homeID, ownerID string, args []string) error {
	return launcher.runner.RunSessionWithOptions(
		ctx, homeID, ownerID, ensureSuperFullAccess(args), superRuntimeLaunchOptions(launcher.options),
	)
}

func (launcher superSessionLauncher) RunSessionReport(ctx context.Context, report sessionmodel.Report, args []string, _ bool) error {
	if launcher.options.profile == "" {
		return launcher.RunSession(ctx, report.AccountID, report.UpstreamAccountID, args)
	}
	if launcher.profiles == nil {
		return errors.New("--profile requires profile-aware Super dispatch")
	}
	target, err := launcher.profiles.ResolveLaunch(ctx, launcher.options.profile)
	if err != nil {
		return err
	}
	if target.AccountID != "" {
		owner := report.UpstreamAccountID
		if owner == "" {
			owner = target.AccountID
		}
		return launcher.RunSession(ctx, target.AccountID, owner, args)
	}
	options := launcher.options
	options.codexArgs = args
	return launchSuperTarget(ctx, launcher.runner, launcher.profiles, target, options, false)
}
