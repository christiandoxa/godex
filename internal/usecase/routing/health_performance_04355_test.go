package routing

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func healthParityAccounts(context.Context) ([]proxymodel.Account, error) {
	return []proxymodel.Account{{ID: "account-a", Enabled: true}, {ID: "account-b", Enabled: true}}, nil
}

func TestProdex04355CoupledWebSocketHealthPenalizesResponsesSelection(t *testing.T) {
	now := time.Unix(10_000, 0)
	router, err := NewRouter(Config{Now: func() time.Time { return now }, Accounts: healthParityAccounts})
	if err != nil {
		t.Fatal(err)
	}
	router.routeHealth[routeHealthKey{accountID: "account-a", route: "websocket"}] = routingentity.RouteHealthScore{
		AccountID: "account-a", Route: "websocket", Score: 4, UpdatedUnix: now.Unix(),
	}
	accounts := []proxymodel.Account{{ID: "account-a", Enabled: true}, {ID: "account-b", Enabled: true}}
	ordered := router.orderCandidates(accounts, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, now)
	if ordered[0].ID != "account-b" {
		t.Fatalf("responses order with coupled websocket health = %v", []string{ordered[0].ID, ordered[1].ID})
	}
}

func TestProdex04355SuccessStreakRecoversTwoThenThreeHealthPoints(t *testing.T) {
	now := time.Unix(20_000, 0)
	router, err := NewRouter(Config{Now: func() time.Time { return now }, Accounts: healthParityAccounts})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	key := routeHealthKey{accountID: "account-a", route: "responses"}
	router.routeHealth[key] = routingentity.RouteHealthScore{AccountID: "account-a", Route: "responses", Score: 5, UpdatedUnix: now.Unix()}
	router.recordRouteSuccess(context.Background(), "account-a", selection)
	if got := router.routeHealth[key].Effective(now); got != 3 {
		t.Fatalf("first success score = %d, want 3", got)
	}
	router.recordRouteSuccess(context.Background(), "account-a", selection)
	if got := router.routeHealth[key].Effective(now); got != 0 {
		t.Fatalf("second success score = %d, want 0", got)
	}
}

func TestProdex04355TransportPrecommitAddsConnectHealthPenalty(t *testing.T) {
	now := time.Unix(30_000, 0)
	router, err := NewRouter(Config{Now: func() time.Time { return now }, Accounts: healthParityAccounts})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	response := &proxymodel.Response{
		StatusCode: http.StatusBadGateway, Body: io.NopCloser(&emptyReader{}),
		PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true, Code: "connect_timeout"},
	}
	outcome, _, err := router.classify(response, "openai")
	if err != nil {
		t.Fatal(err)
	}
	router.recordRouteOutcome(context.Background(), "account-a", selection, response, outcome)
	got := router.routeHealth[routeHealthKey{accountID: "account-a", route: "responses"}].Effective(now)
	if got != 5 {
		t.Fatalf("connect transport health score = %d, want 5", got)
	}
	bad := router.routeMemory[routeMemoryKey{accountID: "account-a", route: "responses", kind: routingentity.RouteMemoryBadPairing}].Effective(now)
	perf := router.routeMemory[routeMemoryKey{accountID: "account-a", route: "responses", kind: routingentity.RouteMemoryPerformance}].Effective(now)
	if bad != 2 || perf != 5 {
		t.Fatalf("transport route memory = bad:%d performance:%d, want 2/5", bad, perf)
	}
}

type emptyReader struct{}

func (*emptyReader) Read([]byte) (int, error) { return 0, io.EOF }
func (*emptyReader) Close() error             { return nil }
