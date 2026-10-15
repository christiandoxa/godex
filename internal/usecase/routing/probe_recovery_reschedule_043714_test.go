package routing

import (
	"context"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex043714BlockedAuthRecoveryIsRevalidatedAfterTooEarlyReceipt(t *testing.T) {
	base := time.Unix(7000, 0)
	clock := base
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	account := proxymodel.Account{ID: "profile-one", Home: "/synthetic", Enabled: true}
	digest := [32]byte{1, 2}
	quota := &causalQuotaReceiptFixture043714{
		valid: true, availability: quotamodel.Availability{Ready: true, Source: quotamodel.SourceLive},
		proof: quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: digest, StartedAt: base},
	}
	router, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil }, QuotaPreflight: quota, Now: func() time.Time { return clock }})
	if err != nil {
		t.Fatal(err)
	}
	response := &proxymodel.Response{StatusCode: 401, AuthCredentialKnown: true, AuthCredentialDigest: digest, AuthRequestStartedAt: base.Add(-time.Second)}
	router.recordRouteOutcome(t.Context(), account.ID, selection, response, responseOutcome{kind: responseAuthFailure})
	router.applyRetryOutcomeForAccount(t.Context(), account, selection, responseOutcome{kind: responseAuthFailure})
	if quota.scheduled != 1 {
		t.Fatalf("first revalidation scheduled %d times", quota.scheduled)
	}
	for i := 0; i < 4; i++ {
		if got := router.requestCandidates([]proxymodel.Account{account}, selection, clock); len(got) != 0 {
			t.Fatalf("unqualified same-second probe released quarantine: %+v", got)
		}
	}
	if quota.scheduled != 1 {
		t.Fatalf("unbounded same-second revalidation storm: %d", quota.scheduled)
	}
	clock = clock.Add(3 * time.Second)
	if got := router.requestCandidates([]proxymodel.Account{account}, selection, clock); len(got) != 0 {
		t.Fatalf("old proof bypassed recency: %+v", got)
	}
	if quota.scheduled != 2 {
		t.Fatalf("no retry scheduled after bounded interval: %d", quota.scheduled)
	}
	quota.proof.StartedAt = clock
	clock = clock.Add(time.Second)
	if got := router.requestCandidates([]proxymodel.Account{account}, selection, clock); len(got) != 1 {
		t.Fatalf("fresh authenticated receipt did not recover profile: %+v", got)
	}
	if quota.scheduled != 2 {
		t.Fatalf("unnecessary new probe after recovery: %d", quota.scheduled)
	}
}
