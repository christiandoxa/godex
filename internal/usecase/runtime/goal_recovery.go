package runtime

import (
	"context"
	"strings"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

const runtimeGoalMonitorPollInterval = 200 * time.Millisecond
const runtimeGoalRecoveryWait = 5 * time.Second

type GoalRecoveryStateReader interface {
	ReadGoalRecoveryState(context.Context, string) (runtimemodel.GoalRecoveryState, bool)
}

// nativeSessionRecoveryProcess is implemented only by gateways that own a
// private app-server companion. Keeping it optional leaves ordinary process
// fakes and explicit remote launches on the existing relaunch path.
type nativeSessionRecoveryProcess interface {
	RecoverNativeSession(context.Context, string, bool) bool
}

type goalRecoveryProfiles interface {
	SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error)
	ResolveLaunch(context.Context, string) (profilemodel.LaunchTarget, error)
}

// RunWithGoalRecoveryMonitor owns the active-to-usage-limited transition
// policy and cancels only the child launch when that transition is fresh.
func (runner *Runner) RunWithGoalRecoveryMonitor(
	ctx context.Context,
	selector string,
	arguments []string,
	options RuntimeLaunchOptions,
	readState func(context.Context) (runtimemodel.GoalRecoveryState, bool),
) (error, bool) {
	if readState == nil {
		return runner.RunWithOptions(ctx, selector, arguments, options), false
	}
	return runner.runWithGoalRecoveryMonitor(ctx, readState, false, func(childContext context.Context) error {
		return runner.RunWithOptions(childContext, selector, arguments, options)
	})
}

func (runner *Runner) runWithGoalRecoveryMonitor(
	ctx context.Context,
	readState func(context.Context) (runtimemodel.GoalRecoveryState, bool),
	nativeOnly bool,
	run func(context.Context) error,
) (error, bool) {
	startedAt := time.Now().UnixMilli()
	sessionID, armed, suppressed := "", false, false
	childContext, cancel := context.WithCancel(ctx)
	defer cancel()
	triggered := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(runtimeGoalMonitorPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-childContext.Done():
				return
			case <-ticker.C:
				if childContext.Err() != nil {
					return
				}
				state, ok := readState(childContext)
				if !ok || state.SessionID == "" {
					continue
				}
				if state.SessionID != sessionID {
					sessionID, armed, suppressed = state.SessionID, false, false
				}
				switch strings.ToLower(strings.TrimSpace(state.Status)) {
				case "active":
					armed, suppressed = true, false
				case "usage_limited":
					if !suppressed && (armed || state.UpdatedAt >= startedAt) {
						if recovery, ok := runner.process.(nativeSessionRecoveryProcess); ok &&
							recovery.RecoverNativeSession(childContext, state.SessionID, true) {
							// The owned companion remains authoritative for deferred,
							// accepted, and ambiguous native decisions. Its own retry
							// interval controls repeated RPC attempts.
							continue
						}
						if nativeOnly {
							continue
						}
						triggered <- struct{}{}
						cancel()
						return
					}
				case "paused", "complete", "completed", "stopped", "cancelled", "canceled", "budget_limited", "budgetlimited":
					// A user stop invalidates a pending automatic recovery.
					armed, suppressed = false, true
				case "blocked":
					// Blocked goals remain eligible for a fresh limit signal.
				default:
					armed = false
				}
			}
		}
	}()
	err := run(childContext)
	cancel()
	<-done
	select {
	case <-triggered:
		return err, true
	default:
		return err, false
	}
}

// runSessionWithNativeRecovery keeps a TUI resume attached to the owned
// companion. Exec resumes and explicit remote launches never provide a
// session ID here, so they retain their existing process path.
func (runner *Runner) runSessionWithNativeRecovery(
	ctx context.Context, sessionID string, run func(context.Context) error,
) error {
	if runner == nil || strings.TrimSpace(sessionID) == "" || runner.goalRecoveryReader == nil {
		return run(ctx)
	}
	if _, ok := runner.process.(nativeSessionRecoveryProcess); !ok {
		return run(ctx)
	}
	err, _ := runner.runWithGoalRecoveryMonitor(ctx, func(readContext context.Context) (runtimemodel.GoalRecoveryState, bool) {
		return runner.ReadGoalRecoveryState(readContext, sessionID)
	}, true, run)
	return err
}

// SharedCodexHome returns the isolated Codex state root used by runtime
// launches. It is a read-only wiring boundary for delivery-owned monitors.
func (runner *Runner) SharedCodexHome() string {
	if runner == nil {
		return ""
	}
	return runner.sharedCodexHome
}

func (runner *Runner) ReadGoalRecoveryState(
	ctx context.Context, sessionID string,
) (runtimemodel.GoalRecoveryState, bool) {
	if runner == nil || runner.goalRecoveryReader == nil {
		return runtimemodel.GoalRecoveryState{}, false
	}
	return runner.goalRecoveryReader.ReadGoalRecoveryState(ctx, sessionID)
}

// WaitForGoalRecoveryProfile owns profile eligibility, quota readiness and
// waiting while a fresh, resumable goal is blocked on its current profile.
func (runner *Runner) WaitForGoalRecoveryProfile(
	ctx context.Context,
	sessionID string,
	readState func(context.Context) (runtimemodel.GoalRecoveryState, bool),
	profiles goalRecoveryProfiles,
	excluded map[string]bool,
	options RuntimeLaunchOptions,
	wait func(context.Context) bool,
) (profilemodel.LaunchTarget, bool) {
	if readState == nil || profiles == nil || strings.TrimSpace(sessionID) == "" {
		return profilemodel.LaunchTarget{}, false
	}
	for ctx.Err() == nil {
		state, ok := readState(ctx)
		if !ok || state.SessionID != sessionID || !goalStatusResumable(state.Status) {
			return profilemodel.LaunchTarget{}, false
		}
		if target, ok, structural := runner.nextGoalRecoveryProfile(ctx, profiles, excluded, options); ok {
			return target, true
		} else if !structural {
			return profilemodel.LaunchTarget{}, false
		}
		if wait == nil {
			wait = waitForGoalRecoveryProfile
		}
		if !wait(ctx) {
			return profilemodel.LaunchTarget{}, false
		}
	}
	return profilemodel.LaunchTarget{}, false
}

func (runner *Runner) nextGoalRecoveryProfile(
	ctx context.Context,
	profiles goalRecoveryProfiles,
	excluded map[string]bool,
	options RuntimeLaunchOptions,
) (profilemodel.LaunchTarget, bool, bool) {
	if runner == nil || runner.accounts == nil || profiles == nil {
		return profilemodel.LaunchTarget{}, false, false
	}
	homes, err := profiles.SessionProfiles(ctx)
	if err != nil {
		return profilemodel.LaunchTarget{}, false, false
	}
	structural := false
	for _, home := range homes {
		if !home.Enabled || home.Provider != "openai" || home.AccountID == "" || excluded[home.AccountID] {
			continue
		}
		target, err := profiles.ResolveLaunch(ctx, home.Name)
		if err != nil || target.AccountID != home.AccountID || target.Provider != "openai" ||
			!strings.EqualFold(strings.TrimSpace(target.Auth), "chatgpt") {
			continue
		}
		structural = true
		if !runner.goalRecoveryAccountReady(ctx, target.AccountID, options) {
			continue
		}
		return target, true, true
	}
	return profilemodel.LaunchTarget{}, false, structural
}

func (runner *Runner) goalRecoveryAccountReady(
	ctx context.Context,
	accountID string,
	options RuntimeLaunchOptions,
) bool {
	if options.SkipQuotaPreflight || runner.quota == nil {
		return true
	}
	autoRedeem := runner.autoRedeem
	if options.AutoRedeem != nil {
		autoRedeem = *options.AutoRedeem
	}
	candidates, err := runner.accounts.LaunchCandidates(ctx, accountID)
	if err != nil || len(candidates) == 0 || ctx.Err() != nil {
		return false
	}
	for _, candidate := range candidates {
		if candidate.ID != accountID || !candidate.Enabled {
			continue
		}
		if autoRedeem {
			return true
		}
		probe := runner.probeCandidateWithRuntimePolicy(
			ctx, candidate, options.UpstreamURL, options.UpstreamNoProxy,
		)
		return probe.err == nil && probe.ready && ctx.Err() == nil
	}
	return false
}

func goalStatusResumable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "blocked", "usage_limited":
		return true
	default:
		return false
	}
}

func waitForGoalRecoveryProfile(ctx context.Context) bool {
	timer := time.NewTimer(runtimeGoalRecoveryWait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}
