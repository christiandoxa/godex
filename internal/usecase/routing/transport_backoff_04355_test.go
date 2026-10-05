package routing

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

const (
	transportAccountA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	transportAccountB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type transportParityGateway struct {
	failA  bool
	owners []string
}

func (gateway *transportParityGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.owners = append(gateway.owners, account.ID)
	if gateway.failA && account.ID == transportAccountA {
		return nil, &url.Error{
			Op: "Post", URL: "https://upstream.test/responses",
			Err: &net.OpError{Op: "read", Err: errors.New("connection reset by peer")},
		}
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}, nil
}

func transportParityAccounts() []proxymodel.Account {
	return []proxymodel.Account{
		{ID: transportAccountA, Home: "/a", Enabled: true},
		{ID: transportAccountB, Home: "/b", Enabled: true},
	}
}

func transportParityRequest(path string) proxymodel.Request {
	return proxymodel.Request{Method: http.MethodPost, Path: path, Header: make(http.Header)}
}

func TestProdex04355TransportBackoffSurvivesRestartAndStaysRouteScoped(t *testing.T) {
	now := time.Unix(90_000, 0)
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	source := func(context.Context) ([]proxymodel.Account, error) {
		return transportParityAccounts(), nil
	}

	firstGateway := &transportParityGateway{failA: true}
	firstRouter, err := NewRouter(Config{
		Gateway: firstGateway, Accounts: source, PreferredAccount: transportAccountA,
		RoutingState: store, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstRouter.Forward(t.Context(), transportParityRequest("/responses"))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(firstGateway.owners, ","); got != transportAccountA+","+transportAccountB {
		t.Fatalf("first transport recovery = %s", got)
	}
	backoffs, err := store.LoadTransportBackoffs(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(backoffs) != 1 || backoffs[0].AccountID != transportAccountA || backoffs[0].Route != "responses" || backoffs[0].UntilUnix != now.Unix()+15 {
		t.Fatalf("persisted transport backoff = %+v", backoffs)
	}

	responsesGateway := &transportParityGateway{}
	responsesRouter, err := NewRouter(Config{
		Gateway: responsesGateway, Accounts: source, PreferredAccount: transportAccountA,
		RoutingState: routingrepo.NewStore(root), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	responses, err := responsesRouter.Forward(t.Context(), transportParityRequest("/responses"))
	if err != nil {
		t.Fatal(err)
	}
	if err := responses.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(responsesGateway.owners, ","); got != transportAccountB {
		t.Fatalf("responses route ignored transport cooldown: %s", got)
	}

	standardGateway := &transportParityGateway{}
	standardRouter, err := NewRouter(Config{
		Gateway: standardGateway, Accounts: source, PreferredAccount: transportAccountA,
		RoutingState: routingrepo.NewStore(root), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	standard, err := standardRouter.Forward(t.Context(), transportParityRequest("/v1/chat/completions"))
	if err != nil {
		t.Fatal(err)
	}
	if err := standard.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(standardGateway.owners, ","); got != transportAccountA {
		t.Fatalf("responses cooldown leaked into standard route: %s", got)
	}
}

func TestProdex04355TransportBackoffDoublesAndCaps(t *testing.T) {
	now := time.Unix(100_000, 0)
	router, err := NewRouter(Config{
		Now:      func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := transportParityRequest("/responses")
	for _, wantSeconds := range []int64{15, 30, 60, 120, 120} {
		router.persistTransportBackoff(t.Context(), transportAccountA, request)
		got := router.transportBackoffRemaining(transportAccountA, request, now)
		if got != time.Duration(wantSeconds)*time.Second {
			t.Fatalf("transport backoff = %s, want %ds", got, wantSeconds)
		}
	}
}

func TestProdex04355TransportBackoffWaitsWhenWholeRoutePoolIsCooling(t *testing.T) {
	start := time.Unix(110_000, 0)
	now := start
	store := routingrepo.NewStore(t.TempDir())
	for _, accountID := range []string{transportAccountA, transportAccountB} {
		if err := store.SetTransportBackoff(t.Context(), routingentity.TransportBackoff{
			AccountID: accountID, Route: "responses", UntilUnix: start.Unix() + 15,
		}, start); err != nil {
			t.Fatal(err)
		}
	}
	var waits []time.Duration
	gateway := &transportParityGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: transportAccountA, RoutingState: store,
		Now: func() time.Time { return now },
		Wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return nil
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return transportParityAccounts(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), transportParityRequest("/responses"))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if len(waits) != 1 || waits[0] != 15*time.Second {
		t.Fatalf("route cooldown waits = %v", waits)
	}
	if got := strings.Join(gateway.owners, ","); got != transportAccountA {
		t.Fatalf("route cooldown dispatched before recovery: %s", got)
	}
	persisted, err := store.LoadTransportBackoffs(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 || persisted[0].AccountID != transportAccountB {
		t.Fatalf("success did not clear only committed route backoff: %+v", persisted)
	}
}

func TestProdex04355HardAffinityBypassesTransportBackoff(t *testing.T) {
	now := time.Unix(120_000, 0)
	gateway := &transportParityGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: transportAccountB, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return transportParityAccounts(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.affinity.remember(t.Context(), transportAccountA, affinityKeys{session: "session-hard-transport"}, now); err != nil {
		t.Fatal(err)
	}
	request := transportParityRequest("/responses")
	request.Body = []byte(`{"session_id":"session-hard-transport"}`)
	router.persistTransportBackoff(t.Context(), transportAccountA, request)

	exchange, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != transportAccountA || strings.Join(gateway.owners, ",") != transportAccountA {
		t.Fatalf("hard affinity rotated during transport cooldown: owner=%q attempts=%v", exchange.Result.AccountID, gateway.owners)
	}
}

type transportPrecommitGateway struct {
	owners []string
}

func (gateway *transportPrecommitGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.owners = append(gateway.owners, account.ID)
	if account.ID == transportAccountA {
		return &proxymodel.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("transport failure")),
			PrecommitFailure: &proxymodel.PrecommitFailure{
				Code: "transport_failure", Transport: true,
			},
		}, nil
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}, nil
}

func TestProdex04355TransportPrecommitDoesNotAlsoCreateGenericRetryBackoff(t *testing.T) {
	now := time.Unix(130_000, 0)
	store := routingrepo.NewStore(t.TempDir())
	gateway := &transportPrecommitGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: transportAccountA, RoutingState: store,
		Now:      func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return transportParityAccounts(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), transportParityRequest("/responses"))
	if err != nil {
		t.Fatal(err)
	}
	if err := exchange.Close(); err != nil {
		t.Fatal(err)
	}
	retryBackoffs, err := store.LoadRetryBackoffs(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(retryBackoffs) != 0 {
		t.Fatalf("transport failure also created generic retry backoff: %+v", retryBackoffs)
	}
	transportBackoffs, err := store.LoadTransportBackoffs(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(transportBackoffs) != 1 || transportBackoffs[0].AccountID != transportAccountA || transportBackoffs[0].Route != "responses" {
		t.Fatalf("transport precommit backoff = %+v", transportBackoffs)
	}
}

func TestProdex04355TransportFailureClassifierMatchesTaggedKinds(t *testing.T) {
	for name, err := range map[string]error{
		"dns":            errors.New("failed to lookup address information"),
		"tls":            errors.New("TLS handshake failed"),
		"ws timeout":     errors.New("websocket handshake timed out"),
		"closed stream":  errors.New("stream closed before response.completed"),
		"typed network":  &net.OpError{Op: "read", Err: errors.New("connection reset")},
		"unexpected eof": io.ErrUnexpectedEOF,
	} {
		if !isTransportFailure(err) {
			t.Errorf("%s was not classified as transport failure: %v", name, err)
		}
	}
	for name, err := range map[string]error{
		"application": errors.New("invalid request"),
		"canceled":    context.Canceled,
	} {
		if isTransportFailure(err) {
			t.Errorf("%s was classified as transport failure: %v", name, err)
		}
	}
}
