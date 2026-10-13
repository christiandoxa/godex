package routing

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type freshTransportFailure04371Gateway struct{ calls int }

func (gateway *freshTransportFailure04371Gateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	gateway.calls++
	return nil, &url.Error{Op: "Get", URL: "http://upstream.invalid", Err: &net.OpError{
		Op:  "dial",
		Err: errors.New("i/o timeout"),
	}}
}

// The 0.437.1 finite transport fallback is superseded for Responses by the
// 0.437.8 request-scoped recovery watchdog. A cooling, still-eligible account
// does not exhaust the request after just one 30-second recovery epoch.
func TestProdex04378FreshTransportFailureExpiresAsWatchdog504(t *testing.T) {
	gateway := &freshTransportFailure04371Gateway{}
	waits := 0
	router, err := NewRouter(Config{
		Gateway: gateway, PrecommitWatchdogBudget: 40 * time.Millisecond,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/synthetic/a", Enabled: true}}, nil
		},
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits++
			if delay <= 0 || delay > maxFreshRecoveryWait {
				t.Errorf("unbounded transport recovery delay %s", delay)
			}
			<-ctx.Done()
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = router.Forward(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses",
		Header: make(http.Header), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	var routeErr *proxymodel.Error
	if !errors.As(err, &routeErr) || routeErr.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("fresh transport error = %v, want watchdog HTTP 504", err)
	}
	if gateway.calls != 1 || waits != 1 {
		t.Fatalf("fresh transport attempts/waits = %d/%d, want 1/1 before watchdog timeout", gateway.calls, waits)
	}
}
