package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

const (
	providerShortcutNotImplementedFormat = "runtime provider shortcut %q is not implemented yet"
	anthropicProviderKind                = "anthropic"
	deepSeekProviderKind                 = "deepseek"
	copilotProviderKind                  = "copilot"
	geminiProviderKind                   = "gemini"
	kiroProviderKind                     = "kiro"
)

func runProfilelessProviderSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	selection runtimemodel.Selection,
	arguments []string,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) error {
	if selection.Provider == kiroProviderKind {
		return providerCredentialRequired(kiroProviderKind)
	}
	keys, err := runner.ProviderAPIKeys(selection.Provider, selection.APIKey)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return providerCredentialRequired(selection.Provider)
	}
	provider, err := externalAPIKeyProvider(selection.Provider, selection.Provider+"-api-key", selection.BaseURL)
	if err != nil {
		return err
	}
	applyProviderSelectionModel(&provider, selection.Model)
	if err := runtimeusecase.ApplyProviderSelectionLimits(&provider, selection.ContextWindow, selection.AutoCompactTokenLimit); err != nil {
		return err
	}
	return runner.RunProviderAPIKeysWithOptions(ctx, "", provider, keys, arguments, launchOptions)
}

func runProviderSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	arguments []string,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) error {
	if selection.Provider != anthropicProviderKind && selection.Provider != copilotProviderKind &&
		selection.Provider != deepSeekProviderKind && selection.Provider != geminiProviderKind &&
		selection.Provider != kiroProviderKind {
		return fmt.Errorf(providerShortcutNotImplementedFormat, selection.Provider)
	}
	target, found, err := profiles.ResolveProviderLaunch(ctx, selection.Provider, selection.Profile)
	if err != nil {
		return err
	}
	if selection.Provider == kiroProviderKind {
		return runKiroProviderSelection(ctx, runner, profiles, selection, target, found, arguments, launchOptions)
	}
	keys, err := runner.ProviderAPIKeys(selection.Provider, selection.APIKey)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return runProviderAPIKeySelection(ctx, runner, providerAPIKeyRequest{
			profiles: profiles, selection: selection, target: target, found: found,
			keys: keys, arguments: arguments, launchOptions: launchOptions,
		})
	}
	if selection.Provider == anthropicProviderKind {
		return runProviderOAuthSelection(ctx, runner, profiles, selection, target, found, arguments, launchOptions)
	}
	return providerCredentialRequired(selection.Provider)
}

type providerAPIKeyRequest struct {
	profiles      launchProfiles
	selection     runtimemodel.Selection
	target        profilemodel.LaunchTarget
	found         bool
	keys          []string
	arguments     []string
	launchOptions runtimeusecase.RuntimeLaunchOptions
}

func runProviderAPIKeySelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	request providerAPIKeyRequest,
) (runErr error) {
	home := ""
	name := request.selection.Provider + "-api-key"
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
	provider, err := externalAPIKeyProvider(request.selection.Provider, name, request.selection.BaseURL)
	if err != nil {
		return err
	}
	applyProviderSelectionModel(&provider, request.selection.Model)
	if err := runtimeusecase.ApplyProviderSelectionLimits(
		&provider, request.selection.ContextWindow, request.selection.AutoCompactTokenLimit,
	); err != nil {
		return err
	}
	if request.found && request.target.AccountID != "" {
		return runner.RunProviderAPIKeysAccountWithOptions(
			ctx, request.target.AccountID, provider, request.keys, request.arguments, request.launchOptions,
		)
	}
	return runner.RunProviderAPIKeysWithOptions(ctx, home, provider, request.keys, request.arguments, request.launchOptions)
}

func runKiroProviderSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	target profilemodel.LaunchTarget,
	found bool,
	arguments []string,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) (runErr error) {
	if !found || target.Provider != kiroProviderKind {
		return providerCredentialRequired(kiroProviderKind)
	}
	release, err := profiles.AcquireLaunch(ctx, target.Name)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	provider := runtimeusecase.KiroProvider(target.Name)
	applyProviderSelectionModel(&provider, selection.Model)
	if err := runtimeusecase.ApplyProviderSelectionLimits(
		&provider, selection.ContextWindow, selection.AutoCompactTokenLimit,
	); err != nil {
		return err
	}
	return runner.RunProviderProfileWithOptions(ctx, target.CodexHome, provider, arguments, launchOptions)
}

func runProviderOAuthSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	target profilemodel.LaunchTarget,
	found bool,
	arguments []string,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
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
	applyProviderSelectionModel(&provider, selection.Model)
	if err := runtimeusecase.ApplyProviderSelectionLimits(&provider, selection.ContextWindow, selection.AutoCompactTokenLimit); err != nil {
		return err
	}
	allowRotate := selection.Profile == ""
	if launchOptions.AllowAutoRotate != nil && !*launchOptions.AllowAutoRotate {
		allowRotate = false
	}
	pool, err := profiles.ProviderLaunchPool(ctx, target.Name, anthropicProviderKind, allowRotate)
	if err != nil {
		return err
	}
	return runProviderPool(ctx, runner, providerPoolRequest{
		profiles: profiles, selected: target, provider: provider, pool: pool,
		arguments: arguments, apiURLOverride: selection.BaseURL, launchOptions: launchOptions,
	})
}

func applyProviderSelectionModel(provider *proxymodel.Provider, model string) {
	if provider == nil {
		return
	}
	if model = strings.TrimSpace(model); model != "" {
		provider.DefaultModel = model
	}
}

func externalAPIKeyProvider(kind, name, baseURL string) (proxymodel.Provider, error) {
	switch kind {
	case anthropicProviderKind:
		return runtimeusecase.AnthropicProvider(name, baseURL), nil
	case deepSeekProviderKind:
		return runtimeusecase.DeepSeekProvider(name, baseURL), nil
	case copilotProviderKind:
		return runtimeusecase.CopilotProvider(name, "", "", baseURL), nil
	case geminiProviderKind:
		return runtimeusecase.GeminiProvider(name, baseURL), nil
	default:
		return proxymodel.Provider{}, fmt.Errorf(providerShortcutNotImplementedFormat, kind)
	}
}

func providerCredentialRequired(kind string) error {
	switch kind {
	case anthropicProviderKind:
		return errors.New("godex run --provider anthropic requires a Claude profile, --api-key, or ANTHROPIC_API_KEY(S)")
	case deepSeekProviderKind:
		return errors.New("godex run --provider deepseek requires --api-key or DEEPSEEK_API_KEY(S)")
	case copilotProviderKind:
		return errors.New("godex run --provider copilot requires an imported Copilot profile, --api-key, or GITHUB_COPILOT_API_KEY(S)")
	case geminiProviderKind:
		return errors.New("godex run --provider gemini requires --api-key, GEMINI_API_KEY(S), or GOOGLE_API_KEY(S)")
	case kiroProviderKind:
		return errors.New("godex run --provider kiro requires an imported Kiro profile from `godex profile import kiro`")
	default:
		return fmt.Errorf(providerShortcutNotImplementedFormat, kind)
	}
}
