package runtime

import (
	"context"
	"errors"
	"strconv"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

type resumeProviderPlan struct {
	kind   string
	direct bool
}

func NewSessionLauncher(runner *runtimeusecase.Runner, profiles launchProfiles) sessionusecase.Launcher {
	return runSessionLauncher{runner: runner, profiles: profiles}
}

func (launcher runSessionLauncher) RunSessionReport(
	ctx context.Context,
	report sessionmodel.Report,
	args []string,
	local bool,
) error {
	if launcher.runner == nil {
		return errors.New("runtime support is not configured")
	}
	args = restoreResumeSessionSettings(args, report)
	if local {
		return launcher.runLocalReport(ctx, report, args)
	}

	selection := launcher.selection
	if selection.URL != "" {
		return runLocalProviderSelection(ctx, launcher.runner, launcher.profiles, selection, args)
	}
	if selection.Provider != "" {
		if launcher.profiles == nil {
			return runProfilelessProviderSelection(ctx, launcher.runner, selection, args, launcher.options)
		}
		return runProviderSelection(ctx, launcher.runner, launcher.profiles, selection, args, launcher.options)
	}
	if selection.Profile != "" {
		if launcher.profiles == nil {
			return errors.New("--profile requires profile-aware runtime dispatch")
		}
		target, err := launcher.profiles.ResolveLaunch(ctx, selection.Profile)
		if err != nil {
			return err
		}
		return launcher.runResolvedTarget(ctx, report, target, args)
	}
	if selection.Account != "" {
		if report.AccountID == "" {
			return errors.New("selected account does not own this session")
		}
		return launcher.runner.RunSessionWithOptions(ctx, report.AccountID, report.UpstreamAccountID, args, launcher.options)
	}
	if _, explicit := runDryConfigOverride(args, "model_provider"); explicit {
		return launcher.runDefaultReport(ctx, report, args)
	}

	plan, err := resumeProviderIdentity(report.ModelProvider)
	if err != nil {
		return err
	}
	if plan.direct {
		return launcher.runDirectReport(ctx, report, args)
	}
	if plan.kind != "" {
		if launcher.profiles == nil {
			auto := selection
			auto.Provider = plan.kind
			return runProfilelessProviderSelection(ctx, launcher.runner, auto, args, launcher.options)
		}
		auto := selection
		auto.Provider = plan.kind
		return runProviderSelection(ctx, launcher.runner, launcher.profiles, auto, args, launcher.options)
	}
	return launcher.runDefaultReport(ctx, report, args)
}

func (launcher runSessionLauncher) runLocalReport(
	ctx context.Context,
	report sessionmodel.Report,
	args []string,
) error {
	if report.Profile != "" && launcher.profiles != nil {
		target, err := launcher.profiles.ResolveLaunch(ctx, report.Profile)
		if err != nil {
			return err
		}
		return launcher.runner.RunHome(ctx, target.CodexHome, args)
	}
	if report.AccountID != "" {
		return launcher.runner.RunLocal(ctx, report.AccountID, args)
	}
	if report.CodexHome != "" {
		return launcher.runner.RunHome(ctx, report.CodexHome, args)
	}
	return errors.New("session home is unavailable")
}

func (launcher runSessionLauncher) runDirectReport(
	ctx context.Context,
	report sessionmodel.Report,
	args []string,
) error {
	if report.Profile != "" && launcher.profiles != nil {
		target, err := launcher.profiles.ResolveLaunch(ctx, report.Profile)
		if err != nil {
			return err
		}
		return launcher.runner.RunDirectProfileWithOptions(ctx, target.CodexHome, args, launcher.options)
	}
	if report.CodexHome != "" {
		return launcher.runner.RunDirectProfileWithOptions(ctx, report.CodexHome, args, launcher.options)
	}
	return errors.New("session home is unavailable")
}

func (launcher runSessionLauncher) runDefaultReport(
	ctx context.Context,
	report sessionmodel.Report,
	args []string,
) error {
	if launcher.profiles != nil {
		target, active, err := launcher.profiles.ActiveLaunch(ctx)
		if err != nil {
			return err
		}
		if active {
			return launcher.runResolvedTarget(ctx, report, target, args)
		}
	}
	if report.AccountID != "" {
		return launcher.runner.RunSessionWithOptions(ctx, report.AccountID, report.UpstreamAccountID, args, launcher.options)
	}
	provider := strings.ToLower(strings.TrimSpace(report.ModelProvider))
	if provider == "godex-local" || provider == "prodex-local" || provider == "amazon-bedrock" || provider == "amazon-bedrock-runtime" {
		if report.CodexHome != "" {
			return launcher.runner.RunDirectProfileWithOptions(ctx, report.CodexHome, args, launcher.options)
		}
	}
	return launcher.runner.RunWithOptions(ctx, "", args, launcher.options)
}

func (launcher runSessionLauncher) runResolvedTarget(
	ctx context.Context,
	report sessionmodel.Report,
	target profilemodel.LaunchTarget,
	args []string,
) error {
	if target.AccountID != "" && target.Provider == "openai" && target.Auth != "api-key" {
		owner := report.UpstreamAccountID
		if owner == "" {
			owner = target.AccountID
		}
		return launcher.runner.RunSessionWithOptions(ctx, target.AccountID, owner, args, launcher.options)
	}
	return runLaunchTarget(ctx, launcher.runner, nil, launcher.profiles, target, args, true, launcher.options)
}

func resumeProviderIdentity(value string) (resumeProviderPlan, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "openai", "godex-openai", "godex-openai-governed-http",
		"prodex-openai", "prodex-openai-governed-http",
		"godex-openai-compatible", "prodex-openai-compatible",
		"godex-local", "prodex-local":
		return resumeProviderPlan{}, nil
	case "amazon-bedrock", "amazon-bedrock-runtime":
		return resumeProviderPlan{direct: true}, nil
	case "godex-anthropic", "prodex-anthropic":
		return resumeProviderPlan{kind: anthropicProviderKind}, nil
	case "godex-copilot", "prodex-copilot":
		return resumeProviderPlan{kind: copilotProviderKind}, nil
	case "godex-deepseek", "prodex-deepseek":
		return resumeProviderPlan{kind: deepSeekProviderKind}, nil
	case "godex-gemini", "prodex-gemini":
		return resumeProviderPlan{kind: geminiProviderKind}, nil
	case "godex-kiro", "prodex-kiro":
		return resumeProviderPlan{kind: kiroProviderKind}, nil
	default:
		return resumeProviderPlan{}, errors.New(
			"resumed session has an unsupported provider identity; configure the matching provider or start a fresh session",
		)
	}
}

func restoreResumeSessionSettings(args []string, report sessionmodel.Report) []string {
	if !codexResumeRequested(args) {
		return args
	}
	result := append([]string(nil), args...)
	modelExplicit := runDryCLIModel(args) != ""
	if _, ok := runDryConfigOverride(args, "model"); ok {
		modelExplicit = true
	}
	_, effortExplicit := runDryConfigOverride(args, "model_reasoning_effort")
	if !modelExplicit && strings.TrimSpace(report.LastModel) != "" {
		result = prependConfigOverride(result, "model", report.LastModel)
	}
	if !effortExplicit && strings.TrimSpace(report.LastReasoningEffort) != "" {
		result = prependConfigOverride(result, "model_reasoning_effort", report.LastReasoningEffort)
	}
	return result
}

func prependConfigOverride(args []string, key, value string) []string {
	result := make([]string, 0, len(args)+2)
	result = append(result, "-c", key+"="+strconv.Quote(strings.TrimSpace(value)))
	return append(result, args...)
}
