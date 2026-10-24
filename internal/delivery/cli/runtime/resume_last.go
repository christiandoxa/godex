package runtime

import (
	"context"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

func nativeResumeLastRequested(args []string) bool {
	command := nativeCommandIndex(args)
	if command < 0 || args[command] != "resume" {
		return false
	}
	for index := command + 1; index < len(args); index++ {
		if args[index] == "--" {
			break
		}
		if nativeOptionTakesValue(args[index]) {
			index++
		} else if args[index] == "--last" {
			return true
		}
	}
	return false
}

func resolveRunResumeLast(
	ctx context.Context, sessions *sessionusecase.Catalog,
	selection runtimemodel.Selection, args []string,
) (runtimemodel.Selection, []string, error) {
	if !nativeResumeLastRequested(args) || sessions == nil {
		return selection, args, nil
	}
	report, err := nativeResumeSettings(ctx, sessions, args)
	if err != nil {
		return selection, nil, err
	}
	args = restoreResumeSessionSettings(args, report)
	if selection.Provider != "" || selection.URL != "" {
		return selection, args, nil
	}
	if _, explicit := runDryConfigOverride(args, "model_provider"); explicit {
		return selection, args, nil
	}
	plan, err := resumeProviderIdentity(report.ModelProvider)
	if err == nil && selection.Profile == "" && selection.Account == "" {
		selection.Provider = plan.kind
	}
	return selection, args, err
}

func nativeResumeSettings(ctx context.Context, sessions *sessionusecase.Catalog, args []string) (sessionmodel.Report, error) {
	selector := "--last"
	if index := findResumeSessionSelector(args, nativeCommandIndex(args)+1); index >= 0 {
		selector = args[index]
	}
	return sessions.NativeResumeSettings(ctx, sessionmodel.Launch{SessionSelector: selector, Arguments: args})
}
