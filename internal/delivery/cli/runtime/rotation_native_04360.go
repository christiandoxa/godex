package runtime

import (
	"context"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

// Prodex 0.436.0's default run path uses the native Codex transport if
// it has no qualifying multi-profile auto-rotation pool. The Super
// overlay, local governance and upstream overrides still require their
// own existing runtime launch path.
func qualifiedOpenAINativeLaunch04360(
	ctx context.Context, profiles launchProfiles, target profilemodel.LaunchTarget,
	options runtimeusecase.RuntimeLaunchOptions,
) (runtimeusecase.RuntimeLaunchOptions, error) {
	if profiles == nil || target.Provider != "openai" || target.Name == "" ||
		options.SuperOverlay || options.SmartContextEnabled ||
		options.PresidioEnabled || options.UpstreamURL != "" || options.UpstreamNoProxy {
		return options, nil
	}
	probe, ok := profiles.(interface {
		RuntimeRotationEligible(context.Context, string) (bool, error)
	})
	if !ok {
		return options, nil
	}
	eligible, err := probe.RuntimeRotationEligible(ctx, target.Name)
	if err != nil {
		return options, err
	}
	if options.AllowAutoRotate != nil && !*options.AllowAutoRotate {
		eligible = false
	}
	options.NativeWithoutProxy = !eligible
	return options, nil
}
