package runtime

import (
	"context"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func runRuntimeLaunchOptions(selection runtimemodel.Selection) runtimeusecase.RuntimeLaunchOptions {
	allowRotate := !selection.NoAutoRotate
	autoRedeem := selection.AutoRedeem
	options := runtimeusecase.RuntimeLaunchOptions{
		SkipQuotaPreflight: selection.SkipQuotaCheck,
		AutoRedeem:         &autoRedeem,
		AllowAutoRotate:    &allowRotate,
		UpstreamNoProxy:    selection.NoProxy,
	}
	if selection.Provider == "" && selection.URL == "" {
		options.UpstreamURL = selection.BaseURL
	}
	return options
}

type runSessionLauncher struct {
	runner    *runtimeusecase.Runner
	profiles  launchProfiles
	selection runtimemodel.Selection
	options   runtimeusecase.RuntimeLaunchOptions
}

func (launcher runSessionLauncher) Run(ctx context.Context, selector string, args []string) error {
	return launcher.runner.RunWithOptions(ctx, selector, args, launcher.options)
}

func (launcher runSessionLauncher) RunLocal(ctx context.Context, selector string, args []string) error {
	return launcher.runner.RunLocal(ctx, selector, args)
}

func (launcher runSessionLauncher) RunSession(ctx context.Context, homeID, ownerID string, args []string) error {
	return launcher.runner.RunSessionWithOptions(ctx, homeID, ownerID, args, launcher.options)
}
