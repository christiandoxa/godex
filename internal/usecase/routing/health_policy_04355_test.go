package routing

import (
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04355HealthPolicyProductionDecayAndCompositeSort(t *testing.T) {
	now := time.Unix(1000, 0)
	router, err := NewRouter(Config{Now: func() time.Time { return now }, Accounts: healthParityAccounts})
	if err != nil {
		t.Fatal(err)
	}
	router.routeHealth[routeHealthKey{accountID: "alpha", route: "responses"}] = routingentity.RouteHealthScore{AccountID: "alpha", Route: "responses", Score: 2, UpdatedUnix: now.Unix()}
	router.routeHealth[routeHealthKey{accountID: "alpha", route: "websocket"}] = routingentity.RouteHealthScore{AccountID: "alpha", Route: "websocket", Score: 4, UpdatedUnix: now.Unix()}
	for _, score := range []routingentity.RouteMemoryScore{
		{AccountID: "alpha", Route: "responses", Kind: routingentity.RouteMemoryBadPairing, Score: 3, UpdatedUnix: now.Unix()},
		{AccountID: "alpha", Route: "websocket", Kind: routingentity.RouteMemoryBadPairing, Score: 2, UpdatedUnix: now.Unix()},
		{AccountID: "alpha", Route: "responses", Kind: routingentity.RouteMemoryPerformance, Score: 8, UpdatedUnix: now.Unix()},
		{AccountID: "alpha", Route: "websocket", Kind: routingentity.RouteMemoryPerformance, Score: 4, UpdatedUnix: now.Unix()},
	} {
		router.routeMemory[routeMemoryKey{accountID: score.AccountID, route: score.Route, kind: score.Kind}] = score
	}
	if got := router.routeCompositeHealthScore("alpha", "responses", now); got != 18 {
		t.Fatalf("composite health = %d, want 18", got)
	}

	bad := routingentity.RouteMemoryScore{AccountID: "alpha", Route: "responses", Kind: routingentity.RouteMemoryBadPairing, Score: 3, UpdatedUnix: now.Unix()}
	perf := routingentity.RouteMemoryScore{AccountID: "alpha", Route: "responses", Kind: routingentity.RouteMemoryPerformance, Score: 3, UpdatedUnix: now.Unix()}
	streak := routingentity.RouteMemoryScore{AccountID: "alpha", Route: "responses", Kind: routingentity.RouteMemorySuccessStreak, Score: 3, UpdatedUnix: now.Unix()}
	if got := bad.Effective(now.Add(180 * time.Second)); got != 2 {
		t.Fatalf("bad-pairing decay = %d", got)
	}
	if got := perf.Effective(now.Add(300 * time.Second)); got != 2 {
		t.Fatalf("performance decay = %d", got)
	}
	if got := streak.Effective(now.Add(300 * time.Second)); got != 2 {
		t.Fatalf("success-streak decay = %d", got)
	}
}

func TestProdex04355LatencyPolicyMatchesTaggedThresholds(t *testing.T) {
	for _, fixture := range []struct {
		ms   uint64
		want uint8
	}{
		{120, 0}, {121, 2}, {300, 2}, {301, 4}, {700, 4}, {701, 7}, {1500, 7}, {1501, 12},
	} {
		if got := healthLatencyPenalty(fixture.ms, quotamodel.RouteKindResponses, "ttfb"); got != fixture.want {
			t.Fatalf("response ttfb %dms penalty = %d, want %d", fixture.ms, got, fixture.want)
		}
	}
	if got := healthLatencyPenalty(181, quotamodel.RouteKindCompact, "connect"); got != 4 {
		t.Fatalf("compact connect penalty = %d", got)
	}
	if got := healthLatencyNextScore(5, 0); got != 3 {
		t.Fatalf("good observation score = %d", got)
	}
	if got := healthLatencyNextScore(1, 7); got != 3 {
		t.Fatalf("poor observation score = %d", got)
	}
	if got := transportHealthPenalty("unexpected_eof"); got != 4 {
		t.Fatalf("unexpected EOF transport penalty = %d", got)
	}
	if got := transportHealthPenalty("tls_handshake"); got != 5 {
		t.Fatalf("TLS transport penalty = %d", got)
	}
}

func TestProdex04355HealthRecoveryDecisionMatchesTaggedStreak(t *testing.T) {
	for _, fixture := range []struct {
		score, streak, wantScore, wantStreak uint8
		retain                               bool
	}{
		{5, 0, 3, 1, true}, {16, 3, 13, 3, true}, {3, 1, 0, 0, false},
	} {
		score, streak, retain := routeHealthRecovery(fixture.score, fixture.streak)
		if score != fixture.wantScore || streak != fixture.wantStreak || retain != fixture.retain {
			t.Fatalf("recovery(%d,%d)=(%d,%d,%t), want (%d,%d,%t)", fixture.score, fixture.streak, score, streak, retain, fixture.wantScore, fixture.wantStreak, fixture.retain)
		}
	}
}
