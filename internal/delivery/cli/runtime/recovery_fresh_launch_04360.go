package runtime

import (
	"context"
	"strings"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

// runParsedWithOptionsWithRecovery preserves the original run path unless
// an explicitly selected managed OpenAI account starts a new headless Codex
// exec session and the installed session catalogue can independently
// verify the resulting persisted rollout.
//
// Exactly one new session, a trusted accepted turn and a structured
// usage-limit marker are required before a single profile retarget.
func runParsedWithOptionsWithRecovery(
	ctx context.Context, runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog, profiles launchProfiles,
	selector string, args []string, opts runtimeusecase.RuntimeLaunchOptions,
) error {
	if runner == nil || sessions == nil || profiles == nil ||
		strings.TrimSpace(selector) == "" ||
		opts.NativeWithoutProxy || opts.UpstreamNoProxy || opts.SuperOverlay ||
		opts.PresidioEnabled ||
		(opts.AllowAutoRotate != nil && !*opts.AllowAutoRotate) ||
		!freshExecInvocation04360(args) {
		return runParsedWithOptions(ctx, runner, sessions, selector, args, opts)
	}
	before, err := sessions.List(ctx, sessionmodel.Query{})
	if err != nil || ctx.Err() != nil {
		return runParsedWithOptions(ctx, runner, sessions, selector, args, opts)
	}
	original := runParsedWithOptions(ctx, runner, sessions, selector, args, opts)
	if original == nil || ctx.Err() != nil || recoveryExitCancelled04360(original) {
		return original
	}
	after, err := sessions.List(ctx, sessionmodel.Query{})
	if err != nil {
		return original
	}
	report, ok := newSessionAfter04360(before, after)
	if !ok || report.AccountID != "" && report.AccountID != selector ||
		report.UpstreamAccountID != "" && report.UpstreamAccountID != selector ||
		report.ModelProvider != "" && report.ModelProvider != "openai" ||
		!goalAllowsRecovery04360(ctx, report.CodexHome, report.ID) {
		return original
	}
	failureClass := freshSessionRecoveryClass04360(ctx, report.Path, report.ID)
	if failureClass == "" {
		return original
	}
	report.AccountID = selector
	report.UpstreamAccountID = selector
	chooser := runSessionLauncher{runner: runner, profiles: profiles, options: opts}
	resumed, ok := retargetCodexExecRecovery04360(args, report.ID)
	if !ok {
		return original
	}
	// For fresh exec, retarget first and restore persisted model/effort
	// from the unique newly created session before a follow-up attempt.
	resumed = restoreResumeSessionSettings(resumed, report)
	resumed = append(resumed, recoveryContinuationPrompt04360)
	return chooser.recoverPersistedSessionThroughPool04360(
		ctx, report, resumed, original, failureClass, sessions.ReleaseRecoveryBinding,
	)
}

func freshExecInvocation04360(args []string) bool {
	index := nativeCommandIndex(args)
	if index < 0 || args[index] != "exec" {
		return false
	}
	nested := nextCommandWord(args, index+1)
	if nested >= 0 {
		switch args[nested] {
		case "resume", "review":
			return false
		}
	}
	_, valid := retargetCodexExecRecovery04360(args, "00000000-0000-4000-8000-000000000001")
	return valid
}
