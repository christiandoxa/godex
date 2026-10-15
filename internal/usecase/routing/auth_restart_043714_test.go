package routing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

// Prodex 0.437.14 carries a historical 401 into the next runtime process,
// even if its previous quota snapshot was fresh. The new runtime must keep
// the owner excluded until a *new* authenticated receipt revalidates it.
func TestProdex043714RestartRestoresOnlyAuthPenaltyUntilVerifiedQuotaRecovery(t *testing.T) {
	now := time.Unix(1600000000, 0)
	digest := [32]byte{1, 2, 3}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	account := proxymodel.Account{ID: "synthetic-one", Home: "/synthetic", Enabled: true}
	repository := routingrepo.NewStore(filepath.Join(t.TempDir(), "routing"))
	quota := &causalQuotaReceiptFixture043714{proof: quotamodel.AuthenticatedReceipt{
		Confirmed: true, CredentialDigest: digest, StartedAt: now.Add(3 * time.Second),
	}, availability: quotamodel.Availability{Ready: true, Source: quotamodel.SourceLive}}
	newRouter := func() *Router {
		router, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{account}, nil
		}, RoutingState: repository, QuotaPreflight: quota, Now: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		return router
	}
	first := newRouter()
	first.recordRouteOutcome(t.Context(), account.ID, selection, &proxymodel.Response{
		StatusCode: 401, AuthCredentialKnown: true, AuthCredentialDigest: digest, AuthRequestStartedAt: now.Add(-time.Second),
	}, responseOutcome{kind: responseAuthFailure})
	first.applyRetryOutcomeForAccount(t.Context(), account, selection, responseOutcome{kind: responseAuthFailure})
	if len(first.candidates([]proxymodel.Account{account}, now)) != 0 {
		t.Fatal("failed auth was initially eligible")
	}
	second := newRouter()
	if len(second.candidates([]proxymodel.Account{account}, now)) != 0 {
		t.Fatal("new process lost inherited auth penalty")
	}
	if len(second.requestCandidates([]proxymodel.Account{account}, selection, now)) != 0 {
		t.Fatal("ready quota cache incorrectly released historical auth penalty")
	}
	if quota.scheduled < 2 {
		t.Fatal("inherited 401 did not schedule authenticated quota revalidation despite ready cache")
	}
	quota.valid = true
	now = now.Add(5 * time.Second)
	if got := second.requestCandidates([]proxymodel.Account{account}, selection, now); len(got) != 1 {
		t.Fatalf("new authenticated receipt did not retire inherited 401: %+v", got)
	}
	third := newRouter()
	if len(third.candidates([]proxymodel.Account{account}, now)) != 1 {
		t.Fatal("cleared auth penalty resurrected on next restart")
	}
}

// A receipt in runtime A cannot clear a newer 401 that runtime B persisted
// while A was still waiting for its quota worker. The store's conditional
// deletion must fail closed, regardless of local memory racing with the other
// process.
func TestProdex043714RestartRejectsOldReceiptAfterNewProcessAuthFailure(t *testing.T) {
	now := time.Unix(1700000000, 0)
	root := filepath.Join(t.TempDir(), "routing")
	repository := routingrepo.NewStore(root)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	account := proxymodel.Account{ID: "profile-one", Home: "/synthetic", Enabled: true}
	quota := &causalQuotaReceiptFixture043714{availability: quotamodel.Availability{Ready: true, Source: quotamodel.SourceLive}}
	construct := func() *Router {
		router, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil },
			RoutingState: repository, QuotaPreflight: quota, Now: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		return router
	}
	digest := [32]byte{3, 2, 1}
	failure := func(router *Router) {
		router.recordRouteOutcome(t.Context(), account.ID, selection, &proxymodel.Response{StatusCode: 401,
			AuthCredentialKnown: true, AuthCredentialDigest: digest, AuthRequestStartedAt: now.Add(-time.Second)},
			responseOutcome{kind: responseAuthFailure})
		router.applyRetryOutcomeForAccount(t.Context(), account, selection, responseOutcome{kind: responseAuthFailure})
	}
	a := construct()
	failure(a)
	b := construct()
	now = now.Add(3 * time.Second)
	failure(b)
	quota.valid = true
	quota.proof = quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: digest, StartedAt: now.Add(time.Second)}
	now = now.Add(2 * time.Second)
	if got := a.requestCandidates([]proxymodel.Account{account}, selection, now); len(got) != 0 {
		t.Fatalf("old process erased newer persisted 401: %+v", got)
	}
	latest := construct()
	if got := latest.candidates([]proxymodel.Account{account}, now); len(got) != 0 {
		t.Fatalf("newer auth penalty disappeared after stale receipt/restart: %+v", got)
	}
}

func TestProdex043714RestartAuth401And403DecayLikeTaggedProductionScores(t *testing.T) {
	for _, tc := range []struct {
		status              int
		activeAt, expiredAt time.Duration
	}{
		{401, 59 * time.Minute, 61 * time.Minute},
		{403, 9 * time.Minute, 11 * time.Minute},
	} {
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			base := time.Unix(1700000000, 0)
			now := base
			account := proxymodel.Account{ID: "profile-one", Home: "/synthetic", Enabled: true}
			store := routingrepo.NewStore(t.TempDir())
			factory := func() *Router {
				router, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil },
					RoutingState: store, Now: func() time.Time { return now }})
				if err != nil {
					t.Fatal(err)
				}
				return router
			}
			first := factory()
			first.recordRouteOutcome(t.Context(), account.ID, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, &proxymodel.Response{
				StatusCode: tc.status, AuthCredentialKnown: true, AuthCredentialDigest: [32]byte{4},
				AuthRequestStartedAt: base.Add(-time.Second)}, responseOutcome{kind: responseAuthFailure})
			now = base.Add(tc.activeAt)
			if got := factory().candidates([]proxymodel.Account{account}, now); len(got) != 0 {
				t.Fatalf("tagged auth health penalty expired prematurely: %+v", got)
			}
			now = base.Add(tc.expiredAt)
			if got := factory().candidates([]proxymodel.Account{account}, now); len(got) != 1 {
				t.Fatalf("tagged auth health penalty never expired: %+v", got)
			}
		})
	}
}

func TestProdex043714RestartAuthClearRetainsDurableGenericRetryBackoff(t *testing.T) {
	base := time.Unix(1700000000, 0)
	now := base
	account := proxymodel.Account{ID: "profile-one", Home: "/synthetic", Enabled: true}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	store := routingrepo.NewStore(t.TempDir())
	quota := &causalQuotaReceiptFixture043714{availability: quotamodel.Availability{Ready: true, Source: quotamodel.SourceLive}}
	create := func() *Router {
		router, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil },
			RoutingState: store, QuotaPreflight: quota, Now: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		return router
	}
	first := create()
	first.persistRetryBackoff(t.Context(), account.ID, 20*time.Minute)
	digest := [32]byte{6, 4, 3}
	first.recordRouteOutcome(t.Context(), account.ID, selection, &proxymodel.Response{StatusCode: 401,
		AuthCredentialKnown: true, AuthCredentialDigest: digest, AuthRequestStartedAt: base.Add(-time.Second)},
		responseOutcome{kind: responseAuthFailure})
	restart := create()
	now = now.Add(5 * time.Second)
	quota.valid = true
	quota.proof = quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: digest, StartedAt: base.Add(2 * time.Second)}
	ranked := restart.requestCandidates([]proxymodel.Account{account}, selection, now)
	if len(ranked) != 1 {
		t.Fatalf("verified receipt did not release auth-specific lease: %+v", ranked)
	}
	if got := freshRetryCandidates(restart, ranked, map[string]bool{account.ID: true}, selection, now); len(got) != 0 {
		t.Fatalf("auth recovery wrongly erased unrelated rate backoff: %+v", got)
	}
	third := create()
	if third.authFailureQuarantined(account.ID, now) || !third.isQuarantined(account.ID, now) {
		t.Fatal("restart resurrected auth or lost independent persisted retry backoff")
	}
	now = base.Add(21 * time.Minute)
	if got := create().candidates([]proxymodel.Account{account}, now); len(got) != 1 {
		t.Fatalf("expired general retry permanently blocked profile: %+v", got)
	}
}

func TestProdex043714RestartAuthStoreRejectsCorruptedPrimaryAndBackup(t *testing.T) {
	now := time.Unix(1700000000, 0)
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	entry := routingentity.AuthBackoff{AccountID: "profile-one", FailedAtMillis: now.UnixMilli(), UntilMillis: now.Add(time.Hour).UnixMilli()}
	if err := store.SetAuthBackoff(t.Context(), entry, now); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "auth-backoff.json")
	payload := []byte(`{"version":1,"backoffs":[{"account_id":"profile-one","failed_at_ms":-1,"until_ms":0}]}`)
	if err := os.WriteFile(directory, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directory+".last-good", payload, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
		RoutingState: store, Now: func() time.Time { return now }})
	if err == nil {
		t.Fatal("new router accepted corrupted auth failure records")
	}
}
