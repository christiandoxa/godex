package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

const maxVerifiedRecoveryProfiles04360 = 32
const runtimeRecoveryRetryInterval04360 = 5 * time.Second

// Prodex 0.436.0 classifies only these three workflow errors as
// eligible to retry a fully exhausted profile pool after a wait.
func recoveryClassRetriesAfterPoolRound04360(class string) bool {
	return class == "rate_limit" || class == "overload" || class == "transport"
}

// Ctrl+C is conveyed to the application context by main's
// signal.NotifyContext. Cancellation always interrupts the wait.
func waitRuntimeRecoveryRound04360(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	timer := time.NewTimer(runtimeRecoveryRetryInterval04360)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func (launcher runSessionLauncher) recoverPersistedSessionThroughPool04360(
	ctx context.Context,
	report sessionmodel.Report,
	resumed []string,
	failed error,
	failureClass string,
	forget func(context.Context, string) error,
) error {
	attempted := map[string]bool{
		report.AccountID:         true,
		report.UpstreamAccountID: true,
	}
	triedThisRound := 0
	for {
		if failureClass == "" || recoveryExitCancelled04360(failed) {
			return failed
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !goalAllowsRecovery04360(ctx, report.CodexHome, report.ID) {
			return failed
		}
		candidate, ok := launcher.recoveryCandidateExcluding04360(ctx, report, attempted)
		if !ok || triedThisRound >= maxVerifiedRecoveryProfiles04360 {
			// Do not wait forever without a single qualified backup.
			// A completed quota-limit or auth error does not recycle.
			if triedThisRound == 0 || !recoveryClassRetriesAfterPoolRound04360(failureClass) {
				return failed
			}
			wait := launcher.recoveryWait
			if wait == nil {
				wait = waitRuntimeRecoveryRound04360
			}
			fmt.Fprintln(os.Stderr,
				"Godex: transient profile pool unavailable; retrying in 5 seconds (Ctrl+C to cancel)")
			if !wait(ctx) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return failed
			}
			attempted = map[string]bool{
				report.AccountID:         true,
				report.UpstreamAccountID: true,
			}
			triedThisRound = 0
			continue
		}
		// A new verified checkpoint is mandatory before every subsequent
		// child. No replay can use evidence retained from an older attempt.
		nextCheckpoint := captureRecoveryCheckpoint04360(report.Path)
		if !nextCheckpoint.valid {
			return failed
		}
		nextGoal := captureGoalTransition04360(ctx, report.CodexHome, report.ID)
		if err := forget(ctx, report.ID); err != nil {
			return errors.Join(failed, fmt.Errorf("session recovery affinity release failed: %w", err))
		}
		attempted[candidate.AccountID] = true
		triedThisRound++
		nextErr := launcher.runner.RunWithOptions(ctx, candidate.AccountID, resumed, launcher.options)
		if nextErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if recoveryExitCancelled04360(nextErr) {
			return nextErr
		}
		failed = nextErr
		failureClass = nextCheckpoint.newAcceptedRecoveryClass04360(ctx, report.ID)
		if failureClass == "" && nextGoal.newUsageLimit04360(ctx) {
			failureClass = "usage_limit"
		}
	}
}
