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
	startedAt := time.Now().UnixMilli()
	sessionID, armed := "", false
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
				state, ok := readState(childContext)
				if !ok || state.SessionID == "" {
					continue
				}
				if state.SessionID != sessionID {
					sessionID, armed = state.SessionID, false
				}
				switch strings.ToLower(strings.TrimSpace(state.Status)) {
				case "active":
					armed = true
				case "usage_limited":
					if armed || state.UpdatedAt >= startedAt {
						triggered <- struct{}{}
						cancel()
						return
					}
				}
			}
		}
	}()
	err := runner.RunWithOptions(childContext, selector, arguments, options)
	cancel()
	<-done
	select {
	case <-triggered:
		return err, true
	default:
		return err, false
	}
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
	case "active", "paused", "blocked", "usage_limited":
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
