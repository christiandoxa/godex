package routing

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type localFailureGateway04371 struct{ calls int }

func (gateway *localFailureGateway04371) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	gateway.calls++
	return nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "proxied request could not be prepared"}
}

func TestFreshLocalGatewayFailureDoesNotRetryOrWait(t *testing.T) {
	gateway := &localFailureGateway04371{}
	waits := 0
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "profile-a", Home: "/profile-a", Enabled: true}}, nil
		},
		Wait: func(context.Context, time.Duration) error {
			waits++
			return errors.New("unexpected retry wait")
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = router.Forward(t.Context(), proxymodel.Request{Header: make(http.Header)})
	var routeErr *proxymodel.Error
	if !errors.As(err, &routeErr) || routeErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("local gateway error = %v, want preserved HTTP 502 error", err)
	}
	if gateway.calls != 1 || waits != 0 {
		t.Fatalf("gateway calls/waits = %d/%d, want one call and no wait", gateway.calls, waits)
	}
}

func TestProxyPreparationErrorMatchIsNarrow(t *testing.T) {
	for _, fixture := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "preparation", err: &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "proxied request could not be prepared"}, want: true},
		{name: "other status", err: &proxymodel.Error{StatusCode: http.StatusConflict, Message: "proxied request could not be prepared"}},
		{name: "other message", err: &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "managed request failed"}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if got := isProxyPreparationError(fixture.err); got != fixture.want {
				t.Fatalf("isProxyPreparationError() = %v, want %v", got, fixture.want)
			}
		})
	}
}
