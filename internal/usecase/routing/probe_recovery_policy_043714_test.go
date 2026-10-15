package routing

import "testing"

func confirmedProdex043714Probe() probeRecoveryFacts043714 {
	return probeRecoveryFacts043714{
		authenticatedSuccess: true, sameCredentials: true,
		startedAt: 120, now: 125,
		authFailurePresent: true, authFailureScore: 12, authFailureUpdatedAt: 100,
		quotaReady: true, previousPresent: true, previousCheckedAt: 110,
		previousFiveHourExhausted: true, previousFiveHourResetAt: 500,
		retryPresent: true, retryUntil: 500, retryUpdatePresent: true, retryUpdatedAtMillis: 110000,
	}
}

// All comparisons below originate in Prodex 0.437.14's compiled Mojo
// runtime_health.mojo decision plus the Rust ABI tests. A false positive here
// can erase a real auth, transport or quota failure and replay an unsafe turn.
func TestProdex043714AuthenticatedProbeRecoveryCausalPolicy(t *testing.T) {
	baseline := confirmedProdex043714Probe()
	if got := probeRecoveryPolicy043714(baseline); got != (probeRecoveryPlan043714{true, true, true}) {
		t.Fatalf("canonical authenticated ready receipt: %+v", got)
	}
	cases := []struct {
		name   string
		mutate func(*probeRecoveryFacts043714)
		want   probeRecoveryPlan043714
	}{
		{"no authenticated receipt", func(f *probeRecoveryFacts043714) { f.authenticatedSuccess = false }, probeRecoveryPlan043714{}},
		{"credential rotated", func(f *probeRecoveryFacts043714) { f.sameCredentials = false }, probeRecoveryPlan043714{}},
		{"probe from future", func(f *probeRecoveryFacts043714) { f.startedAt = 130 }, probeRecoveryPlan043714{}},
		{"newer snapshot", func(f *probeRecoveryFacts043714) { f.previousCheckedAt = 121 }, probeRecoveryPlan043714{}},
		{"newer quota backoff", func(f *probeRecoveryFacts043714) { f.retryUpdatedAtMillis = 120000 }, probeRecoveryPlan043714{}},
		{"newer auth failure", func(f *probeRecoveryFacts043714) { f.authFailureUpdatedAt = 120 }, probeRecoveryPlan043714{false, true, true}},
		{"no auth failure", func(f *probeRecoveryFacts043714) { f.authFailurePresent = false }, probeRecoveryPlan043714{false, true, true}},
		{"zero auth score", func(f *probeRecoveryFacts043714) { f.authFailureScore = 0 }, probeRecoveryPlan043714{false, true, true}},
		{"exhausted quota", func(f *probeRecoveryFacts043714) { f.quotaReady = false }, probeRecoveryPlan043714{true, false, true}},
		{"mismatched rate backoff", func(f *probeRecoveryFacts043714) { f.retryUntil = 501 }, probeRecoveryPlan043714{true, false, true}},
		{"no previous quota snapshot", func(f *probeRecoveryFacts043714) { f.previousPresent = false }, probeRecoveryPlan043714{true, false, true}},
		{"no retry backoff", func(f *probeRecoveryFacts043714) { f.retryPresent = false }, probeRecoveryPlan043714{true, false, true}},
		{"weekly reset", func(f *probeRecoveryFacts043714) {
			f.previousFiveHourExhausted = false
			f.previousWeeklyExhausted = true
			f.previousWeeklyResetAt = 500
		}, probeRecoveryPlan043714{true, true, true}},
		{"no exhausted window", func(f *probeRecoveryFacts043714) { f.previousFiveHourExhausted = false }, probeRecoveryPlan043714{true, false, true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := baseline
			tc.mutate(&facts)
			if got := probeRecoveryPolicy043714(facts); got != tc.want {
				t.Fatalf("plan=%+v want=%+v", got, tc.want)
			}
		})
	}
}
