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
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

type transportBackoffGateway struct {
	owners       []string
	failAccountA bool
}

func (gateway *transportBackoffGateway) Execute(_ context.Context, _ proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.owners = append(gateway.owners, account.ID)
	if account.ID == "account-a" && gateway.failAccountA {
		gateway.failAccountA = false
		return nil, &url.Error{Op: "Get", URL: "https://upstream.test/responses", Err: &net.OpError{Op: "dial", Err: errors.New("connection reset")}}
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}, nil
}

func TestTransportFailureBackoffPersistsAndStaysRouteScoped(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0)
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true},
	}
	accountSource := func(context.Context) ([]proxymodel.Account, error) {
		return append([]proxymodel.Account(nil), accounts...), nil
	}
	firstGateway := &transportBackoffGateway{failAccountA: true}
	firstRouter, err := NewRouter(Config{
		Gateway: firstGateway, PreferredAccount: "account-a", RoutingState: store,
		Now: func() time.Time { return now }, Accounts: accountSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstRouter.Forward(ctx, transportBackoffRequest(quotamodel.RouteKindResponses))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(firstGateway.owners, ","); got != "account-a,account-b" {
		t.Fatalf("first request owners = %q", got)
	}
	backoffs, err := store.LoadTransportBackoffs(ctx, now)
	if err != nil || len(backoffs) != 1 || backoffs[0].AccountID != "account-a" ||
		backoffs[0].Route != "responses" || backoffs[0].UntilUnix != now.Add(routingentity.InitialTransportBackoffDuration).Unix() {
		t.Fatalf("persisted transport backoffs = %+v, error = %v", backoffs, err)
	}

	secondGateway := &transportBackoffGateway{}
	secondRouter, err := NewRouter(Config{
		Gateway: secondGateway, PreferredAccount: "account-a", RoutingState: routingrepo.NewStore(root),
		Now: func() time.Time { return now }, Accounts: accountSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	standard, err := secondRouter.Forward(ctx, transportBackoffRequest(quotamodel.RouteKindStandard))
	if err != nil {
		t.Fatal(err)
	}
	if err := standard.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(secondGateway.owners, ","); got != "account-a" {
		t.Fatalf("different route ignored transport backoff: %q", got)
	}
	responses, err := secondRouter.Forward(ctx, transportBackoffRequest(quotamodel.RouteKindResponses))
	if err != nil {
		t.Fatal(err)
	}
	defer responses.Close()
	if got := strings.Join(secondGateway.owners, ","); got != "account-a,account-b" {
		t.Fatalf("backed-off route owners after restart = %q", got)
	}
}

func TestTransportBackoffDoublesToTaggedMaximum(t *testing.T) {
	now := time.Unix(100, 0)
	router, err := NewRouter(Config{Now: func() time.Time { return now }, Accounts: func(context.Context) ([]proxymodel.Account, error) {
		return nil, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	want := []time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second, 120 * time.Second, 120 * time.Second}
	for _, duration := range want {
		router.persistTransportBackoff(context.Background(), "account-a", selection)
		got := router.transportBackoffRemaining("account-a", selection, now)
		if got != duration {
			t.Fatalf("transport backoff = %s, want %s", got, duration)
		}
	}
}

func TestIsTransportFailureUsesTypedNetworkCauses(t *testing.T) {
	for name, err := range map[string]error{
		"network reset":  &url.Error{Op: "Get", URL: "https://upstream.test", Err: &net.OpError{Op: "dial", Err: errors.New("connection reset")}},
		"read timeout":   &net.OpError{Op: "read", Err: osTimeoutError{}},
		"unexpected eof": io.ErrUnexpectedEOF,
	} {
		if !isTransportFailure(err) {
			t.Errorf("%s was not classified as transport failure", name)
		}
	}
	for name, err := range map[string]error{
		"application error":   errors.New("invalid request"),
		"caller cancellation": context.Canceled,
	} {
		if isTransportFailure(err) {
			t.Errorf("%s was classified as transport failure", name)
		}
	}
}

type osTimeoutError struct{}

func (osTimeoutError) Error() string   { return "timeout" }
func (osTimeoutError) Timeout() bool   { return true }
func (osTimeoutError) Temporary() bool { return true }

func transportBackoffRequest(route quotamodel.RouteKind) proxymodel.Request {
	return proxymodel.Request{
		Header:         make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: route},
	}
}
