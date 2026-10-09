package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

// liveGoalRecovery04361 observes only the goal database row for the UUID
// delivered by Codex's trusted SessionStart hook. Historical usage_limited rows
// stay inert until an active row was observed for this launch.
type liveGoalRecovery04361 struct {
	marker    *sessionStartMarker04360
	sessionID string
	read      func(context.Context, string) (runtimemodel.GoalRecoveryState, bool)
}

func (monitor *liveGoalRecovery04361) readState(ctx context.Context) (runtimemodel.GoalRecoveryState, bool) {
	if monitor == nil || monitor.marker == nil || monitor.read == nil {
		return runtimemodel.GoalRecoveryState{}, false
	}
	id := monitor.marker.ID()
	if id == "" {
		return runtimemodel.GoalRecoveryState{}, false
	}
	monitor.sessionID = id
	state, ok := monitor.read(ctx, id)
	if !ok || state.SessionID != "" && state.SessionID != id {
		return runtimemodel.GoalRecoveryState{}, false
	}
	state.SessionID = id
	return state, true
}

func (monitor *liveGoalRecovery04361) session() string {
	if monitor == nil {
		return ""
	}
	return monitor.sessionID
}

func relaunchLiveGoal04361(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	profiles launchProfiles,
	selector string,
	args []string,
	opts runtimeusecase.RuntimeLaunchOptions,
	sessionID string,
	readState func(context.Context) (runtimemodel.GoalRecoveryState, bool),
	original error,
) error {
	if strings.TrimSpace(sessionID) == "" || sessions == nil || profiles == nil {
		return original
	}
	profileSource, ok := profiles.(interface {
		SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error)
		ResolveLaunch(context.Context, string) (profilemodel.LaunchTarget, error)
	})
	if !ok {
		return original
	}
	canonicalSelector, selectorOK := canonicalRecoveryAccountID04360(ctx, profileSource, selector)
	if !selectorOK {
		return original
	}
	report := sessionmodel.Report{
		ID:                sessionID,
		CodexHome:         runner.SharedCodexHome(),
		AccountID:         canonicalSelector,
		UpstreamAccountID: canonicalSelector,
		ModelProvider:     "openai",
	}
	if resolved, err := sessions.Resolve(ctx, sessionID); err == nil {
		if resolved.CodexHome == "" {
			resolved.CodexHome = report.CodexHome
		}
		if resolved.AccountID == "" {
			resolved.AccountID = canonicalSelector
		}
		if resolved.UpstreamAccountID == "" {
			resolved.UpstreamAccountID = resolved.AccountID
		}
		report = resolved
	}
	if report.AccountID != "" {
		if canonical, ok := canonicalRecoveryAccountID04360(ctx, profileSource, report.AccountID); ok {
			report.AccountID = canonical
		} else if strings.EqualFold(strings.TrimSpace(report.AccountID), strings.TrimSpace(selector)) {
			report.AccountID = canonicalSelector
		} else {
			return original
		}
	}
	if report.UpstreamAccountID != "" {
		if canonical, ok := canonicalRecoveryAccountID04360(ctx, profileSource, report.UpstreamAccountID); ok {
			report.UpstreamAccountID = canonical
		}
	}
	excluded := map[string]bool{
		report.AccountID:         true,
		report.UpstreamAccountID: true,
	}
	candidate, ok := runner.WaitForGoalRecoveryProfile(
		ctx, sessionID, readState, profileSource, excluded, opts, nil,
	)
	if !ok {
		return original
	}
	if err := sessions.ReleaseRecoveryBinding(ctx, sessionID); err != nil {
		return errors.Join(original, fmt.Errorf("session recovery affinity release failed: %w", err))
	}
	resumed := retargetGoalRecoveryArgs04361(args, sessionID)
	resumed = restoreResumeSessionSettings(resumed, report)
	if !codexArgsIncludeGoalResume04361(resumed) {
		resumed = append(resumed, "/goal resume")
	}
	return runner.RunWithOptions(ctx, candidate.AccountID, resumed, opts)
}

func retargetGoalRecoveryArgs04361(args []string, sessionID string) []string {
	if freshExecInvocation04360(args) {
		if resumed, ok := retargetCodexExecRecovery04360(args, sessionID); ok {
			return resumed
		}
	}
	result := appendRecoveryOptions04360(nil, args)
	return append(result, "resume", sessionID)
}

func codexArgsIncludeGoalResume04361(args []string) bool {
	for _, arg := range args {
		if strings.EqualFold(strings.TrimSpace(arg), "/goal resume") {
			return true
		}
	}
	return false
}
