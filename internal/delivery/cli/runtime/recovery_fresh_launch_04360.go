package runtime

import (
	"context"
	"os"
	"runtime"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

// runParsedWithOptionsWithRecovery preserves the original run path unless a
// managed OpenAI launch exposes a fresh goal transition or a verified failed
// headless exec session.
//
// Exactly one new session, a trusted accepted turn and a structured
// usage-limit marker are required before a single profile retarget.
func runParsedWithOptionsWithRecovery(
	ctx context.Context, runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog, profiles launchProfiles,
	selector string, args []string, opts runtimeusecase.RuntimeLaunchOptions,
) error {
	fresh := freshExecInvocation04360(args)
	liveGoalLaunch := liveGoalLaunch04361(args)
	if runner == nil || sessions == nil || profiles == nil ||
		strings.TrimSpace(selector) == "" ||
		opts.NativeWithoutProxy || opts.SuperOverlay ||
		opts.PresidioEnabled ||
		(opts.AllowAutoRotate != nil && !*opts.AllowAutoRotate) ||
		(!liveGoalLaunch || !fresh && strings.TrimSpace(runner.SharedCodexHome()) == "") {
		return runParsedWithOptions(ctx, runner, sessions, selector, args, opts)
	}
	var before []sessionmodel.Report
	if fresh {
		var err error
		before, err = sessions.List(ctx, sessionmodel.Query{})
		if err != nil || ctx.Err() != nil {
			return runParsedWithOptions(ctx, runner, sessions, selector, args, opts)
		}
	}
	// Prodex 0.436.0 uses a native trusted SessionStart hook to
	// disambiguate the child it launched. Keep a private, per-run marker
	// and fall back to unique-session discovery if the hook is unavailable.
	var marker *sessionStartMarker04360
	execArgs := args
	if monitor, monitorErr := newSessionStartMarker04360(); monitorErr == nil {
		marker = monitor
		defer marker.Close()
		if fresh || strings.TrimSpace(runner.SharedCodexHome()) != "" {
			if exe, exeErr := os.Executable(); exeErr == nil {
				execArgs = marker.codexHookArgumentsForOS04360(args, exe, runtime.GOOS)
				if home := goalRecoveryProfileHome04360(ctx, profiles, selector); home != "" {
					execArgs = addRuntimeGoalNotify04360(home, execArgs, exe, marker.path)
				}
			}
		}
	}
	original := error(nil)
	triggered := false
	if marker != nil && strings.TrimSpace(runner.SharedCodexHome()) != "" {
		monitor := &liveGoalRecovery04361{
			marker: marker, read: runner.ReadGoalRecoveryState,
			resolve: func(resolveCtx context.Context, id string) (sessionmodel.Report, error) {
				return sessions.Resolve(resolveCtx, id)
			},
		}
		original, triggered = runner.RunWithGoalRecoveryMonitor(
			ctx, selector, execArgs, opts, monitor.readState,
		)
		if triggered {
			return relaunchLiveGoal04361(
				ctx, runner, sessions, profiles, selector, args, opts, monitor.session(), monitor.readState, original,
			)
		}
	} else {
		original = runParsedWithOptions(ctx, runner, sessions, selector, execArgs, opts)
	}
	if original == nil || ctx.Err() != nil || recoveryExitCancelled04360(original) {
		return original
	}
	if !fresh {
		return original
	}
	canonicalSelector, selectorOK := canonicalRecoveryAccountID04360(ctx, profiles, selector)
	if !selectorOK {
		return original
	}
	after, err := sessions.List(ctx, sessionmodel.Query{})
	if err != nil {
		return original
	}
	markerID := ""
	if marker != nil {
		markerID = marker.ID()
	}
	report, ok := newSessionAfterMarker04360(before, after, markerID)
	if !ok || report.AccountID != "" && report.AccountID != canonicalSelector ||
		report.UpstreamAccountID != "" && report.UpstreamAccountID != canonicalSelector ||
		report.ModelProvider != "" && report.ModelProvider != "openai" ||
		!goalAllowsRecovery04360(ctx, report.CodexHome, report.ID) {
		return original
	}
	failureClass := freshSessionRecoveryClass04360(ctx, report.Path, report.ID)
	if failureClass == "" {
		return original
	}
	report.AccountID = canonicalSelector
	report.UpstreamAccountID = canonicalSelector
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

func goalRecoveryProfileHome04360(
	ctx context.Context, profiles launchProfiles, selector string,
) string {
	if profiles == nil {
		return ""
	}
	if source, ok := profiles.(interface {
		SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error)
		ResolveLaunch(context.Context, string) (profilemodel.LaunchTarget, error)
	}); ok {
		if strings.TrimSpace(selector) != "" {
			if homes, err := source.SessionProfiles(ctx); err == nil {
				for _, home := range homes {
					if home.AccountID != selector && home.Name != selector {
						continue
					}
					target, err := source.ResolveLaunch(ctx, home.Name)
					if err == nil {
						return target.CodexHome
					}
				}
			}
		}
		if target, active, err := profiles.ActiveLaunch(ctx); err == nil && active {
			return target.CodexHome
		}
	}
	return ""
}

func liveGoalLaunch04361(args []string) bool {
	if freshExecInvocation04360(args) {
		return true
	}
	index := nativeCommandIndex(args)
	if index < 0 {
		return true
	}
	switch args[index] {
	case "app-server", "exec-server", "mcp-server", "resume", "fork", "review", "queue",
		"logout", "login", "mcp", "features", "completion", "debug", "config",
		"delete", "archive", "unarchive", "version", "--version":
		return false
	default:
		return true
	}
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
