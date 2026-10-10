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

func TestProdex04371FreshTransportFailureReturnsBounded503(t *testing.T) {
	now := time.Unix(100, 0)
	gateway := &freshTransportFailure04371Gateway{}
	waits := 0
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/synthetic/a", Enabled: true}}, nil
		},
		Wait: func(_ context.Context, delay time.Duration) error {
			waits++
			now = now.Add(delay)
			return nil
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
	if !errors.As(err, &routeErr) || routeErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("fresh transport error = %v, want bounded HTTP 503", err)
	}
	if gateway.calls != 1 || waits != 1 {
		t.Fatalf("fresh transport attempts/waits = %d/%d, want one attempt and one recovery wait", gateway.calls, waits)
	}
}
