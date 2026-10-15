package routing

import "math"

// probeRecoveryFacts043714 mirrors the 18 ordered facts accepted by
// Prodex 0.437.14's compiled prodex_runtime_probe_recovery_v1.
// Only an actual authenticated HTTP receipt may set authenticatedSuccess;
// a persisted DTO, quota cache, or failed/unrelated probe never qualifies.
type probeRecoveryFacts043714 struct {
	authenticatedSuccess, sameCredentials bool
	startedAt, now                        int64
	authFailurePresent                    bool
	authFailureScore                      uint32
	authFailureUpdatedAt                  int64
	quotaReady                            bool
	previousPresent                       bool
	previousCheckedAt                     int64
	previousFiveHourExhausted             bool
	previousFiveHourResetAt               int64
	previousWeeklyExhausted               bool
	previousWeeklyResetAt                 int64
	retryPresent                          bool
	retryUntil                            int64
	retryUpdatePresent                    bool
	retryUpdatedAtMillis                  int64
}

type probeRecoveryPlan043714 struct {
	clearAuthFailure  bool
	clearQuotaBackoff bool
	acceptObservation bool
}

func probeRecoveryPolicy043714(f probeRecoveryFacts043714) probeRecoveryPlan043714 {
	verified := f.authenticatedSuccess && f.sameCredentials && f.startedAt <= f.now
	maxStartedMillis := int64(0)
	if f.startedAt > 0 {
		if f.startedAt > math.MaxInt64/1000 {
			maxStartedMillis = math.MaxInt64
		} else {
			maxStartedMillis = f.startedAt * 1000
		}
	}
	noNewerBackoff := !f.retryUpdatePresent || f.retryUpdatedAtMillis < maxStartedMillis
	accept := verified && (!f.previousPresent || f.previousCheckedAt <= f.startedAt) &&
		(!f.retryPresent || noNewerBackoff)
	matchReset := (f.previousFiveHourExhausted && f.previousFiveHourResetAt == f.retryUntil) ||
		(f.previousWeeklyExhausted && f.previousWeeklyResetAt == f.retryUntil)
	return probeRecoveryPlan043714{
		acceptObservation: accept,
		clearAuthFailure:  accept && f.authFailurePresent && f.authFailureScore > 0 && f.authFailureUpdatedAt < f.startedAt,
		clearQuotaBackoff: accept && f.quotaReady && f.previousPresent && f.previousCheckedAt <= f.startedAt &&
			f.retryPresent && matchReset && noNewerBackoff,
	}
}
