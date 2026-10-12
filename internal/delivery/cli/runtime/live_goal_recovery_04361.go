package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

// liveGoalRecovery04361 observes the goal row and newly appended structured
// recovery evidence for the UUID delivered by Codex's trusted SessionStart hook.
type liveGoalRecovery04361 struct {
	marker             *sessionStartMarker04360
	sessionID          string
	read               func(context.Context, string) (runtimemodel.GoalRecoveryState, bool)
	resolve            func(context.Context, string) (sessionmodel.Report, error)
	checkpoint         recoveryCheckpoint04360
	checkpointPath     string
	scanOffset         int64
	accepted           bool
	scanRetryAt        time.Time
	pendingClass       string
	goalRevision       int64
	pendingRevision    int64
	suppressedRevision int64
	activitySeen       bool
}

func (monitor *liveGoalRecovery04361) readState(ctx context.Context) (runtimemodel.GoalRecoveryState, bool) {
	if monitor == nil || monitor.marker == nil || monitor.read == nil {
		return runtimemodel.GoalRecoveryState{}, false
	}
	id := monitor.marker.ID()
	if id == "" {
		return runtimemodel.GoalRecoveryState{}, false
	}
	if monitor.sessionID != id {
		monitor.sessionID = id
		monitor.checkpoint = recoveryCheckpoint04360{}
		monitor.checkpointPath = ""
		monitor.scanOffset = 0
		monitor.accepted = false
		monitor.scanRetryAt = time.Time{}
		monitor.pendingClass = ""
		monitor.goalRevision = 0
		monitor.pendingRevision = 0
		monitor.suppressedRevision = 0
		monitor.activitySeen = false
	}
	state, ok := monitor.read(ctx, id)
	if ok && state.SessionID != "" && state.SessionID != id {
		return runtimemodel.GoalRecoveryState{}, false
	}
	stopped := false
	if ok {
		previousRevision := monitor.goalRevision
		monitor.goalRevision = state.UpdatedAt
		if previousRevision != 0 && state.UpdatedAt != previousRevision && monitor.activitySeen {
			monitor.suppressedRevision = state.UpdatedAt
		}
		if monitor.pendingClass != "" && state.UpdatedAt != monitor.pendingRevision {
			monitor.pendingClass = ""
			monitor.activitySeen = true
			monitor.suppressedRevision = state.UpdatedAt
		}
		if goalStatusStopsAutomaticRecovery04377(state.Status) {
			monitor.pendingClass = ""
			monitor.activitySeen = true
			monitor.suppressedRevision = state.UpdatedAt
			stopped = true
		}
	}
	if monitor.resolve != nil {
		if report, err := monitor.resolve(ctx, id); err == nil && report.Path != "" {
			if monitor.checkpointPath != report.Path {
				monitor.checkpoint = captureRecoveryCheckpoint04360(report.Path)
				monitor.checkpointPath = report.Path
				monitor.scanOffset = 0
				monitor.accepted = false
				monitor.scanRetryAt = time.Time{}
				monitor.pendingClass = ""
				monitor.pendingRevision = monitor.goalRevision
				monitor.activitySeen = false
				monitor.suppressedRevision = 0
			}
			var class string
			if !monitor.scanRetryAt.After(time.Now()) {
				var nextOffset int64
				var accepted bool
				var activity bool
				var scanned bool
				class, nextOffset, accepted, activity, scanned = monitor.checkpoint.scanAcceptedRecoveryWithActivity04377(
					ctx, id, monitor.scanOffset, monitor.accepted,
				)
				if scanned {
					monitor.scanOffset = nextOffset
					monitor.accepted = accepted
					monitor.scanRetryAt = time.Time{}
					if activity {
						monitor.pendingClass = ""
						monitor.activitySeen = true
						monitor.suppressedRevision = monitor.goalRevision
					}
					if class != "" && !stopped {
						monitor.pendingClass = class
						monitor.pendingRevision = monitor.goalRevision
						monitor.activitySeen = false
					}
				} else if monitor.checkpoint.valid {
					monitor.scanRetryAt = time.Now().Add(runtimeRecoveryRetryInterval04360)
				}
			}
			if !stopped && monitor.pendingClass != "" &&
				(!ok || goalStatusResumable04361(state.Status)) &&
				(!ok || state.UpdatedAt == monitor.pendingRevision) &&
				(!ok || !monitor.activitySeen || state.UpdatedAt != monitor.suppressedRevision) {
				state = runtimemodel.GoalRecoveryState{
					SessionID: id,
					Status:    "usage_limited",
					UpdatedAt: time.Now().UnixMilli(),
				}
				ok = true
			}
		}
	}
	if !ok {
		return runtimemodel.GoalRecoveryState{}, false
	}
	if stopped {
		state.SessionID = id
		return state, true
	}
	if monitor.activitySeen && strings.EqualFold(strings.TrimSpace(state.Status), "usage_limited") {
		// Do not let a stale persisted limit retrigger after newer native work.
		state.Status = "blocked"
	}
	state.SessionID = id
	return state, true
}

func goalStatusResumable04361(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "paused", "blocked", "usage_limited":
		return true
	default:
		return false
	}
}

func goalStatusStopsAutomaticRecovery04377(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paused", "complete", "completed", "stopped", "cancelled", "canceled", "budget_limited", "budgetlimited":
		return true
	default:
		return !goalStatusResumable04361(status)
	}
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
	nextCheckpoint := captureRecoveryCheckpoint04360(report.Path)
	nextGoal := captureGoalTransition04360(ctx, report.CodexHome, sessionID)
	nextErr := runner.RunWithOptions(ctx, candidate.AccountID, resumed, opts)
	if nextErr == nil || ctx.Err() != nil || recoveryExitCancelled04360(nextErr) {
		return nextErr
	}
	failureClass := nextCheckpoint.newAcceptedRecoveryClass04360(ctx, sessionID)
	if failureClass == "" && nextGoal.newUsageLimit04360(ctx) {
		failureClass = "usage_limit"
	}
	if failureClass == "" {
		return nextErr
	}
	launcher := runSessionLauncher{runner: runner, profiles: profiles, options: opts}
	attempted := map[string]bool{report.AccountID: true, report.UpstreamAccountID: true, candidate.AccountID: true}
	return launcher.recoverPersistedSessionThroughPoolStarting04360(
		ctx, report, resumed, nextErr, failureClass, sessions.ReleaseRecoveryBinding, attempted,
	)
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
