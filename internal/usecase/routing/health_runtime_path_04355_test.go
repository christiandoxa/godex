package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type latencyParityGateway struct {
	now          *time.Time
	connect      time.Duration
	stream       bool
	precommitted bool
	firstRead    time.Duration
}

func (gateway *latencyParityGateway) Execute(_ context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	*gateway.now = gateway.now.Add(gateway.connect)
	header := make(http.Header)
	if gateway.stream {
		header.Set("Content-Type", "text/event-stream")
		return &proxymodel.Response{
			StatusCode:          http.StatusOK,
			Header:              header,
			FirstEventCommitted: gateway.precommitted,
			Body: &latencyParityBody{now: gateway.now, firstDelay: gateway.firstRead,
				payload: []byte("data: {\"type\":\"response.completed\"}\n\n")},
		}, nil
	}
	header.Set("Content-Type", "application/json")
	return &proxymodel.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(&emptyReader{})}, nil
}

type latencyParityBody struct {
	now        *time.Time
	firstDelay time.Duration
	payload    []byte
	read       bool
}

func (body *latencyParityBody) Read(buffer []byte) (int, error) {
	if body.read {
		return 0, io.EOF
	}
	body.read = true
	*body.now = body.now.Add(body.firstDelay)
	return copy(buffer, body.payload), nil
}
func (*latencyParityBody) Close() error { return nil }

func TestProdex04355ForwardRecordsConnectLatencyPerformance(t *testing.T) {
	now := time.Unix(100_000, 0)
	gateway := &latencyParityGateway{now: &now, connect: 301 * time.Millisecond}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/responses",
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = exchange.Close()
	key := routeMemoryKey{accountID: "account-a", route: "responses", kind: routingentity.RouteMemoryPerformance}
	if got := router.routeMemory[key].Effective(now); got != 2 {
		t.Fatalf("connect performance score = %d, want 2", got)
	}
}

func TestProdex04355ForwardRecordsResponsesTTFBAndStreamComplete(t *testing.T) {
	now := time.Unix(110_000, 0)
	gateway := &latencyParityGateway{now: &now, connect: 100 * time.Millisecond, stream: true, precommitted: true, firstRead: 301 * time.Millisecond}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/responses",
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(exchange.Result.Response.Body); err != nil {
		t.Fatal(err)
	}
	_ = exchange.Close()
	key := routeMemoryKey{accountID: "account-a", route: "responses", kind: routingentity.RouteMemoryPerformance}
	if got := router.routeMemory[key].Effective(now); got != 3 {
		t.Fatalf("ttfb/complete performance score = %d, want 3", got)
	}
}

func TestProdex04355GlobalHealthContributesToCandidateSort(t *testing.T) {
	now := time.Unix(120_000, 0)
	router, err := NewRouter(Config{Now: func() time.Time { return now }, Accounts: healthParityAccounts})
	if err != nil {
		t.Fatal(err)
	}
	router.routeHealth[routeHealthKey{accountID: "account-a", route: "global"}] = routingentity.RouteHealthScore{
		AccountID: "account-a", Route: "global", Score: 2, UpdatedUnix: now.Unix(),
	}
	ordered := router.orderCandidates(
		[]proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}, {ID: "account-b", Home: "/b", Enabled: true}},
		quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, now,
	)
	if ordered[0].ID != "account-b" {
		t.Fatalf("global health order = %s,%s", ordered[0].ID, ordered[1].ID)
	}
}

type transportFailureParityGateway struct{}

func (*transportFailureParityGateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, errors.New("connection refused")
}

func TestProdex04355BoundTransportErrorUsesExactHealthMemoryPenalties(t *testing.T) {
	now := time.Unix(140_000, 0)
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, Gateway: &transportFailureParityGateway{},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	_, err = router.forwardBound(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/responses", QuotaSelection: selection,
	}, []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, "account-a", &affinityKeys{})
	if err == nil {
		t.Fatal("transport failure unexpectedly succeeded")
	}
	health := router.routeHealth[routeHealthKey{accountID: "account-a", route: "responses"}].Effective(now)
	bad := router.routeMemory[routeMemoryKey{accountID: "account-a", route: "responses", kind: routingentity.RouteMemoryBadPairing}].Effective(now)
	perf := router.routeMemory[routeMemoryKey{accountID: "account-a", route: "responses", kind: routingentity.RouteMemoryPerformance}].Effective(now)
	if health != 5 || bad != 2 || perf != 5 {
		t.Fatalf("transport penalties = health:%d bad:%d performance:%d, want 5/2/5", health, bad, perf)
	}
	if remaining := router.transportBackoffRemaining("account-a", selection, now); remaining <= 0 {
		t.Fatalf("transport backoff = %v, want active", remaining)
	}
}
