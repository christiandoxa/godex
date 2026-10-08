package runtime

import (
	"context"
	"errors"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

const recoveryContinuationPrompt04360 = "Continue the interrupted task from the persisted session. Preserve completed work and do not repeat completed tool calls."

// RunSessionReportWithRecovery closes the known-session Codex child-exit
// boundary: never run again merely because an error string looks retryable.
// An accepted, newly appended, structured usage-limit signal is required.
// The fallback owner must be an eligible, independently authenticated
// profile and the old durable session binding must be released first.
//
// A single bounded continuation is attempted; another failure is returned
// rather than replaying a prompt or looping over transient errors.
func (launcher runSessionLauncher) RunSessionReportWithRecovery(
	ctx context.Context,
	report sessionmodel.Report,
	args []string,
	local bool,
	bindingForget func(context.Context, string) error,
) error {
	if !launcher.recoveryEligible04360(report, args, local, bindingForget) {
		return launcher.RunSessionReport(ctx, report, args, local)
	}
	checkpoint := captureRecoveryCheckpoint04360(report.Path)
	goalBefore := captureGoalTransition04360(ctx, report.CodexHome, report.ID)
	// No verified read position means no safe way to distinguish an old
	// error from this attempt's error.
	if !checkpoint.valid {
		return launcher.RunSessionReport(ctx, report, args, local)
	}
	initial := launcher.RunSessionReport(ctx, report, args, local)
	if initial == nil || ctx.Err() != nil || recoveryExitCancelled04360(initial) {
		return initial
	}
	resumed, ok := retargetCodexExecRecovery04360(
		restoreResumeSessionSettings(args, report), report.ID,
	)
	if !ok {
		return initial
	}
	resumed = append(resumed, recoveryContinuationPrompt04360)
	verified := checkpoint.newAcceptedRecoveryClass04360(ctx, report.ID) != "" ||
		goalBefore.newUsageLimit04360(ctx)
	return launcher.recoverPersistedSessionThroughPool04360(
		ctx, report, resumed, initial, verified, bindingForget,
	)
}

func (launcher runSessionLauncher) recoveryEligible04360(
	report sessionmodel.Report, args []string, local bool,
	forget func(context.Context, string) error,
) bool {
	if local || launcher.runner == nil || launcher.profiles == nil || forget == nil ||
		report.AccountID == "" || report.UpstreamAccountID == "" ||
		launcher.selection.Account != "" || launcher.selection.Profile != "" ||
		launcher.selection.Provider != "" || launcher.selection.URL != "" ||
		launcher.selection.NoAutoRotate || launcher.options.UpstreamNoProxy {
		return false
	}
	if launcher.options.AllowAutoRotate != nil && !*launcher.options.AllowAutoRotate {
		return false
	}
	plan, err := resumeProviderIdentity(report.ModelProvider)
	if err != nil || plan.kind != "" || plan.direct {
		return false
	}
	_, ok := retargetCodexExecRecovery04360(args, report.ID)
	return ok
}

func (launcher runSessionLauncher) recoveryCandidate04360(
	ctx context.Context, report sessionmodel.Report,
) (profilemodel.LaunchTarget, bool) {
	return launcher.recoveryCandidateExcluding04360(ctx, report, nil)
}

func (launcher runSessionLauncher) recoveryCandidateExcluding04360(
	ctx context.Context, report sessionmodel.Report, attempted map[string]bool,
) (profilemodel.LaunchTarget, bool) {
	source, ok := launcher.profiles.(interface {
		SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error)
	})
	if !ok {
		return profilemodel.LaunchTarget{}, false
	}
	homes, err := source.SessionProfiles(ctx)
	if err != nil || len(homes) < 2 || ctx.Err() != nil {
		return profilemodel.LaunchTarget{}, false
	}
	// Enforce the tagged multi-profile quota-compatible requirement.
	for _, home := range homes {
		if !home.Enabled || home.Provider != "openai" || home.AccountID == "" ||
			attempted[home.AccountID] ||
			home.AccountID == report.AccountID || home.AccountID == report.UpstreamAccountID {
			continue
		}
		target, err := launcher.profiles.ResolveLaunch(ctx, home.Name)
		if err != nil || ctx.Err() != nil {
			continue
		}
		if target.AccountID == home.AccountID && target.Provider == "openai" &&
			strings.EqualFold(strings.TrimSpace(target.Auth), "chatgpt") {
			return target, true
		}
	}
	return profilemodel.LaunchTarget{}, false
}

func recoveryExitCancelled04360(err error) bool {
	type exitCoder interface{ ExitCode() int }
	var status exitCoder
	return errors.As(err, &status) && status.ExitCode() == 130
}
