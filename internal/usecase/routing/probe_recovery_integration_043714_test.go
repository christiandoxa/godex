package routing

import (
	"context"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type causalQuotaReceiptFixture043714 struct {
	proof        quotamodel.AuthenticatedReceipt
	availability quotamodel.Availability
	valid        bool
	scheduled    int
}

func (f *causalQuotaReceiptFixture043714) AvailabilityForRoute(_ context.Context, _ accountentity.Account, _ quotamodel.Selection) (quotamodel.Availability, error) {
	return quotamodel.Availability{Ready: true}, nil
}
func (f *causalQuotaReceiptFixture043714) ScheduleAuthenticatedRecoveryProbe(_ context.Context, _ accountentity.Account, _ quotamodel.Selection) error {
	f.scheduled++
	return nil
}
func (f *causalQuotaReceiptFixture043714) AuthenticatedRecoveryReceipt(_ string, _ quotamodel.Selection) (quotamodel.Availability, quotamodel.AuthenticatedReceipt, bool) {
	return f.availability, f.proof, f.valid
}

// The 0.437.14 bug is an empty pool even after a *confirmed same-credential*
// quota response. The route may only retire a strictly older auth-specific
// quarantine; unrelated 429/transport cooldowns remain, and no replay is
// authorized by a mere cached availability DTO.
func TestProdex043714AuthenticatedQuotaRecoveryOnlyRetiresOlderMatchingAuthQuarantine(t *testing.T) {
	base := time.Unix(3000, 0)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	account := proxymodel.Account{ID: "profile-a", Home: "/synthetic", Enabled: true}
	digest := [32]byte{1, 2, 3}
	cases := []struct {
		name              string
		proofValid        bool
		ready             bool
		receiptDigest     [32]byte
		receiptStart      time.Time
		extraPenalty      bool
		expectedCandidate bool
		expectAuth        bool
	}{
		{name: "confirmed same-credential", proofValid: true, ready: true, receiptDigest: digest, receiptStart: base.Add(3 * time.Second), expectedCandidate: true},
		{name: "unknown cached quota", ready: true, receiptDigest: digest, receiptStart: base.Add(3 * time.Second), expectAuth: true},
		{name: "different credentials", proofValid: true, ready: true, receiptDigest: [32]byte{9}, receiptStart: base.Add(3 * time.Second), expectAuth: true},
		{name: "quota still exhausted", proofValid: true, receiptDigest: digest, receiptStart: base.Add(3 * time.Second), expectAuth: true},
		{name: "proof older than auth failure", proofValid: true, ready: true, receiptDigest: digest, receiptStart: base.Add(-time.Second), expectAuth: true},
		{name: "unrelated quarantine protected", proofValid: true, ready: true, receiptDigest: digest, receiptStart: base.Add(3 * time.Second), extraPenalty: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := base
			quota := &causalQuotaReceiptFixture043714{
				proof:        quotamodel.AuthenticatedReceipt{CredentialDigest: tc.receiptDigest, StartedAt: tc.receiptStart, Confirmed: tc.proofValid},
				availability: quotamodel.Availability{Ready: tc.ready, Source: quotamodel.SourceLive}, valid: tc.proofValid,
			}
			router, err := NewRouter(Config{QuotaPreflight: quota, Now: func() time.Time { return clock }, Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil }})
			if err != nil {
				t.Fatal(err)
			}
			response := &proxymodel.Response{StatusCode: 401, AuthCredentialKnown: true, AuthCredentialDigest: digest, AuthRequestStartedAt: base.Add(-time.Second)}
			router.recordRouteOutcome(t.Context(), account.ID, selection, response, responseOutcome{kind: responseAuthFailure})
			router.applyRetryOutcomeForAccount(t.Context(), account, selection, responseOutcome{kind: responseAuthFailure})
			if quota.scheduled != 1 {
				t.Fatalf("auth failure did not schedule one background revalidation: %d", quota.scheduled)
			}
			if tc.extraPenalty {
				router.quarantineAccount(account.ID, 2*time.Minute)
			}
			if got := router.candidates([]proxymodel.Account{account}, clock); len(got) != 0 {
				t.Fatal("profile was selected before authenticated proof")
			}
			clock = base.Add(5 * time.Second)
			got := router.requestCandidates([]proxymodel.Account{account}, selection, clock)
			// General retry leases are enforced by the *send* gate, not
			// necessarily the rank-only requestCandidates projection.
			sending := freshRetryCandidates(router, got, map[string]bool{account.ID: true}, selection, clock)
			if (len(sending) == 1) != tc.expectedCandidate {
				t.Fatalf("untrusted proof or unrelated retry changed dispatch eligibility: ranked=%+v sending=%+v expected=%v", got, sending, tc.expectedCandidate)
			}
			// Non-auth quarantine must remain even when auth proof succeeds;
			// it is never a reason to clear an unrelated transport cooldown.
			if tc.extraPenalty && !router.isQuarantined(account.ID, clock) {
				t.Fatal("unrelated retry lease vanished")
			}
			if tc.expectAuth && !router.authFailureQuarantined(account.ID, clock) {
				t.Fatal("untrusted or out-of-order proof removed auth failure")
			}
		})
	}
}
