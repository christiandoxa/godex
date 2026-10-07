package runtime

import (
	"context"
	"errors"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
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
	if options.provider != "" || options.localURL != "" {
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
