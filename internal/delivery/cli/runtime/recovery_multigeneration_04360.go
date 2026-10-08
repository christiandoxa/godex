package runtime

import (
	"context"
	"errors"
	"fmt"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

// A fixed, bounded pass through distinct eligible profiles mirrors the
// profile-pool exhaustion stage of Prodex 0.436.0 recovery. Every child
// generation is independently monitored from a fresh pre-child
// checkpoint, with no reused acceptance evidence from an older turn.
//
// This is intentionally NOT the tagged source's open-ended transient
// wait/retry scheduler; recycling a fully exhausted pool safely requires
// an independent availability signal and cancellable wait cycle.
const maxVerifiedRecoveryProfiles04360 = 32

func (launcher runSessionLauncher) recoverPersistedSessionThroughPool04360(
	ctx context.Context,
	report sessionmodel.Report,
	resumed []string,
	failed error,
	verified bool,
	forget func(context.Context, string) error,
) error {
	attempted := map[string]bool{
		report.AccountID:         true,
		report.UpstreamAccountID: true,
	}
	for generation := 0; generation < maxVerifiedRecoveryProfiles04360; generation++ {
		if !verified || ctx.Err() != nil || recoveryExitCancelled04360(failed) {
			return failed
		}
		if !goalAllowsRecovery04360(ctx, report.CodexHome, report.ID) {
			return failed
		}
		candidate, ok := launcher.recoveryCandidateExcluding04360(ctx, report, attempted)
		if !ok {
			return failed
		}
		// An invalid/unavailable checkpoint means subsequent retry evidence
		// could not be attributed to this child. Refuse to launch.
		nextCheckpoint := captureRecoveryCheckpoint04360(report.Path)
		if !nextCheckpoint.valid {
			return failed
		}
		nextGoal := captureGoalTransition04360(ctx, report.CodexHome, report.ID)
		if err := forget(ctx, report.ID); err != nil {
			return errors.Join(failed, fmt.Errorf("session recovery affinity release failed: %w", err))
		}
		attempted[candidate.AccountID] = true
		// The same canonical exec-resume plan is safe across attempts:
		// it carries no original prompt and only refers to the persisted
		// thread. The runner rechecks account state and quota each time.
		nextErr := launcher.runner.RunWithOptions(ctx, candidate.AccountID, resumed, launcher.options)
		if nextErr == nil {
			return nil
		}
		if ctx.Err() != nil || recoveryExitCancelled04360(nextErr) {
			return nextErr
		}
		failed = nextErr
		verified = nextCheckpoint.newAcceptedRecoveryClass04360(ctx, report.ID) != "" ||
			nextGoal.newUsageLimit04360(ctx)
	}
	return failed
}
