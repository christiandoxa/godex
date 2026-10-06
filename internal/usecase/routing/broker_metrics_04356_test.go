package routing

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type brokerMetricsGateway struct{}

func (*brokerMetricsGateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return &proxymodel.Response{
		StatusCode: http.StatusNoContent,
		Header:     make(http.Header),
		Body:       io.NopCloser(&emptyBrokerMetricsReader{}),
	}, nil
}

type emptyBrokerMetricsReader struct{}

func (*emptyBrokerMetricsReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestProdex04356ContinuationLifecycleTransitionValuesMatchTaggedPolicy(t *testing.T) {
	now := int64(100)
	status := continuationStatus{kind: "response"}

	status = continuationTouch(status, now)
	if status.state != continuationWarm || status.confidence != 1 || status.lastTouchedAt != 100 {
		t.Fatalf("touch = %#v", status)
	}

	status = continuationVerify(status, now)
	if status.state != continuationVerified || status.confidence != 3 ||
		status.lastTouchedAt != 101 || status.lastVerifiedAt != 101 ||
		status.successCount != 1 || status.failureCount != 0 {
		t.Fatalf("verify = %#v", status)
	}

	status = continuationMarkSuspect(status, now)
	if status.state != continuationSuspect || status.confidence != 2 ||
		status.lastTouchedAt != 102 || status.lastNotFoundAt != 102 ||
		status.notFoundStreak != 1 || status.failureCount != 1 {
		t.Fatalf("first suspect = %#v", status)
	}

	status = continuationMarkSuspect(status, now)
	if status.state != continuationDead || status.confidence != 1 ||
		status.lastTouchedAt != 103 || status.lastNotFoundAt != 103 ||
		status.notFoundStreak != 2 || status.failureCount != 2 {
		t.Fatalf("second suspect = %#v", status)
	}

	status = continuationVerify(status, now)
	if status.state != continuationVerified || status.confidence != 3 ||
		status.lastTouchedAt != 104 || status.lastVerifiedAt != 104 ||
		status.notFoundStreak != 0 || status.lastNotFoundAt != 0 ||
		status.successCount != 2 || status.failureCount != 0 {
		t.Fatalf("reverified = %#v", status)
	}
}

func TestProdex04356BrokerMetricsSnapshotUsesLiveRoutingState(t *testing.T) {
	now := time.Unix(10_000, 0)
	router, err := NewRouter(Config{
		Gateway: &brokerMetricsGateway{},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()

	release, acquired := router.tryAcquireProfileInflight(
		"account-a",
		proxymodel.Request{Path: "/responses"},
		false,
	)
	if !acquired {
		t.Fatal("profile inflight was not acquired")
	}

	router.mu.Lock()
	router.quarantine["retry"] = quarantineState{until: now.Add(20 * time.Second)}
	router.quarantine["auth"] = quarantineState{until: now.Add(20 * time.Second), authFailure: true}
	router.routeHealth[routeHealthKey{accountID: "account-a", route: "global"}] =
		routingentity.RouteHealthScore{AccountID: "account-a", Route: "global", Score: 1, UpdatedUnix: now.Unix()}
	router.routeHealth[routeHealthKey{accountID: "account-a", route: "responses"}] =
		routingentity.RouteHealthScore{AccountID: "account-a", Route: "responses", Score: 2, UpdatedUnix: now.Unix()}
	router.mu.Unlock()

	router.transportMu.Lock()
	router.transportBackoffs[routeHealthKey{accountID: "account-a", route: "responses"}] =
		routingentity.TransportBackoff{AccountID: "account-a", Route: "responses", UntilUnix: now.Add(20 * time.Second).Unix()}
	router.transportBackoffs[routeHealthKey{accountID: "account-a", route: "standard"}] =
		routingentity.TransportBackoff{AccountID: "account-a", Route: "standard", UntilUnix: now.Add(-time.Second).Unix()}
	router.transportMu.Unlock()

	router.routeCircuitMu.Lock()
	router.routeCircuits[routeHealthKey{accountID: "account-a", route: "compact"}] =
		routingentity.RouteCircuit{
			AccountID: "account-a", Route: "compact",
			UntilUnix: now.Add(20 * time.Second).Unix(), StageUpdatedUnix: now.Unix(),
		}
	router.routeCircuitMu.Unlock()

	router.previousResponseFailureMu.Lock()
	router.previousResponseFailures[previousResponseFailureKey{
		accountID: "account-a", responseKey: affinityDigest("previous", "resp-a"), route: "responses",
	}] = routingentity.PreviousResponseFailure{
		AccountID: "account-a", ResponseKey: affinityDigest("previous", "resp-a"),
		Route: "responses", Score: 2, UpdatedUnix: now.Unix(),
	}
	router.previousResponseFailures[previousResponseFailureKey{
		accountID: "account-a", responseKey: affinityDigest("previous", "resp-b"), route: "standard",
	}] = routingentity.PreviousResponseFailure{
		AccountID: "account-a", ResponseKey: affinityDigest("previous", "resp-b"),
		Route: "standard", Score: 1, UpdatedUnix: now.Unix(),
	}
	router.previousResponseFailureMu.Unlock()

	router.affinity.mu.Lock()
	router.affinity.statuses["response"] = continuationStatus{
		kind: "response", state: continuationVerified, confidence: 3,
		lastTouchedAt: now.Add(-continuationVerifiedStale).Unix(), lastVerifiedAt: now.Add(-continuationVerifiedStale).Unix(),
		successCount: 1,
	}
	router.affinity.statuses["turn"] = continuationStatus{
		kind: "turn_state", state: continuationSuspect, confidence: 1,
		lastTouchedAt: now.Unix(), lastNotFoundAt: now.Unix(), notFoundStreak: 1, failureCount: 2,
	}
	router.affinity.statuses["session"] = continuationStatus{
		kind: "session_id", state: continuationDead,
		lastTouchedAt: now.Unix(), lastNotFoundAt: now.Unix(), notFoundStreak: 2, failureCount: 3,
	}
	router.affinity.mu.Unlock()

	snapshot := router.BrokerMetricsSnapshot()
	if snapshot.ProfileInflight["account-a"] != 2 ||
		snapshot.ProfileInflightAdmissionsTotal != 1 ||
		snapshot.RetryBackoffs != 1 ||
		snapshot.TransportBackoffs != 1 ||
		snapshot.RouteCircuits != 1 ||
		snapshot.DegradedProfiles != 1 ||
		snapshot.DegradedRoutes != 1 {
		t.Fatalf("routing broker counters = %#v", snapshot)
	}
	if snapshot.PreviousResponseContinuity.NegativeCacheEntries.Responses != 1 ||
		snapshot.PreviousResponseContinuity.NegativeCacheFailures.Responses != 2 ||
		snapshot.PreviousResponseContinuity.NegativeCacheEntries.Standard != 1 ||
		snapshot.PreviousResponseContinuity.NegativeCacheFailures.Standard != 1 {
		t.Fatalf("previous-response continuity = %#v", snapshot.PreviousResponseContinuity)
	}
	if snapshot.Continuations.ResponseBindings != 1 ||
		snapshot.Continuations.TurnStateBindings != 1 ||
		snapshot.Continuations.SessionIDBindings != 1 ||
		snapshot.Continuations.Verified != 1 ||
		snapshot.Continuations.Suspect != 1 ||
		snapshot.Continuations.Dead != 1 ||
		snapshot.Continuations.FailureCounts.TurnState != 2 ||
		snapshot.Continuations.FailureCounts.SessionID != 3 ||
		snapshot.Continuations.NotFoundStreaks.TurnState != 1 ||
		snapshot.Continuations.NotFoundStreaks.SessionID != 2 ||
		snapshot.Continuations.StaleVerifiedBindings.Response != 1 {
		t.Fatalf("continuation metrics = %#v", snapshot.Continuations)
	}

	release()
	after := router.BrokerMetricsSnapshot()
	if len(after.ProfileInflight) != 0 || after.ProfileInflightReleasesTotal != 1 ||
		after.ProfileInflightReleaseUnderflowsTotal != 0 {
		t.Fatalf("released profile inflight = %#v", after)
	}
}
