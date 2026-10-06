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

func TestPreviousResponseFailureReleasesAffinityAtThresholdAndExcludesProfile(t *testing.T) {
	const accountA = "0123456789abcdef0123456789abcdef"
	const accountB = "fedcba9876543210fedcba9876543210"
	now := time.Unix(1_000_000, 0)
	homeA, homeB := t.TempDir(), t.TempDir()
	for _, home := range []string{homeA, homeB} {
		if err := os.Chmod(home, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store := routingrepo.NewStore(t.TempDir())
	accounts := []proxymodel.Account{
		{ID: accountA, Home: homeA, Enabled: true},
		{ID: accountB, Home: homeB, Enabled: true},
	}
	router, err := NewRouter(Config{
		Bindings: store, RoutingState: store, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := affinityKeys{previous: "resp-dead", session: "session-a"}
	queryKeys := keys
	if err := router.affinity.remember(t.Context(), accountA, keys, now); err != nil {
		t.Fatal(err)
	}
	router.affinity.rememberResponseTurnStateForHome(t.Context(), keys.previous, accountA, homeA, "turn-a", now)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	account := accounts[0]

	if err := router.notePreviousResponseNotFound(t.Context(), account, "resp-dead", selection, &keys); err != nil {
		t.Fatal(err)
	}
	if owner, err := router.affinity.owner(t.Context(), queryKeys, now); err != nil || owner != accountA {
		t.Fatalf("owner after first miss = %q, %v", owner, err)
	}
	if score := router.previousResponseFailureScore(accountA, "resp-dead", selection, now); score != 1 {
		t.Fatalf("score after first miss = %d, want 1", score)
	}

	if err := router.notePreviousResponseNotFound(t.Context(), account, "resp-dead", selection, &keys); err != nil {
		t.Fatal(err)
	}
	if owner, err := router.affinity.owner(t.Context(), queryKeys, now); err != nil || owner != "" {
		t.Fatalf("owner after threshold miss = %q, %v", owner, err)
	}
	if score := router.previousResponseFailureScore(accountA, "resp-dead", selection, now); score != 2 {
		t.Fatalf("score after threshold miss = %d, want 2", score)
	}
	bad := router.routeMemory[routeMemoryKey{accountID: accountA, route: "responses", kind: routingentity.RouteMemoryBadPairing}].Effective(now)
	if bad != 1 {
		t.Fatalf("bad-pairing after threshold miss = %d, want 1", bad)
	}
	turnKey := affinityDigest("previous", "resp-dead")
	if _, err := os.Lstat(filepath.Join(homeA, ".godex-turn-state", turnKey+".json")); !os.IsNotExist(err) {
		t.Fatalf("stale turn-state sidecar remains: %v", err)
	}

	request := proxymodel.Request{Body: []byte(`{"previous_response_id":"resp-dead"}`), QuotaSelection: selection}
	ordered := router.requestCandidatesForRequest(accounts, request, now)
	if len(ordered) != 1 || ordered[0].ID != accountB {
		t.Fatalf("threshold candidates = %#v", ordered)
	}
}

func TestPreviousResponseFailureCandidateExclusionIsRouteScoped(t *testing.T) {
	const body = `{"previous_response_id":"resp-route"}`
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/synthetic/a", Enabled: true},
		{ID: "account-b", Home: "/synthetic/b", Enabled: true},
	}
	router, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) {
		return accounts, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_000_000, 0)
	router.recordPreviousResponseFailure(context.Background(), "account-a", "resp-route", quotamodel.Selection{
		RouteKind: quotamodel.RouteKindResponses,
	}, now)
	router.recordPreviousResponseFailure(context.Background(), "account-a", "resp-route", quotamodel.Selection{
		RouteKind: quotamodel.RouteKindResponses,
	}, now)

	responses := router.requestCandidatesForRequest(accounts, proxymodel.Request{
		Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}, now)
	if len(responses) != 1 || responses[0].ID != "account-b" {
		t.Fatalf("Responses candidates = %#v", responses)
	}
	standard := router.requestCandidatesForRequest(accounts, proxymodel.Request{
		Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindStandard},
	}, now)
	if len(standard) != 2 {
		t.Fatalf("Standard candidates = %#v", standard)
	}
}

func TestPreviousResponseFailureCacheEvictsOldestAtCapacity(t *testing.T) {
	const accountID = "0123456789abcdef0123456789abcdef"
	now := time.Unix(1_000_000, 0)
	router := &Router{previousResponseFailures: make(map[previousResponseFailureKey]routingentity.PreviousResponseFailure)}
	var oldest previousResponseFailureKey
	for index := range routingentity.MaxPreviousResponseFailures {
		responseKey := fmt.Sprintf("%064x", index+1)
		key := previousResponseFailureKey{accountID: accountID, responseKey: responseKey, route: "responses"}
		if index == 0 {
			oldest = key
		}
		router.previousResponseFailures[key] = routingentity.PreviousResponseFailure{
			AccountID: accountID, ResponseKey: responseKey, Route: "responses", Score: 1,
			UpdatedUnix: now.Unix() - int64(routingentity.MaxPreviousResponseFailures-index),
		}
	}

	if score := router.recordPreviousResponseFailure(context.Background(), accountID, "resp-new", quotamodel.Selection{
		RouteKind: quotamodel.RouteKindResponses,
	}, now); score != 1 {
		t.Fatalf("new response failure score = %d, want 1", score)
	}
	newKey := previousResponseFailureKey{
		accountID: accountID, responseKey: affinityDigest("previous", "resp-new"), route: "responses",
	}
	if len(router.previousResponseFailures) != routingentity.MaxPreviousResponseFailures {
		t.Fatalf("cached failure count = %d, want %d", len(router.previousResponseFailures), routingentity.MaxPreviousResponseFailures)
	}
	if _, exists := router.previousResponseFailures[oldest]; exists {
		t.Fatal("oldest cached failure was not evicted")
	}
	if _, exists := router.previousResponseFailures[newKey]; !exists {
		t.Fatal("newest cached failure was dropped")
	}
}
