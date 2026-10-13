package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type awaitingRecoveryGateway04378 struct {
	attempts  atomic.Int32
	cancelled atomic.Bool
}

func (gateway *awaitingRecoveryGateway04378) Execute(
	ctx context.Context, _ proxymodel.Request, _ proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.attempts.Add(1)
	<-ctx.Done()
	gateway.cancelled.Store(true)
	return nil, ctx.Err()
}

func TestProdex04378ProxyPrecommitWatchdogReturnsTyped504(t *testing.T) {
	gateway := &awaitingRecoveryGateway04378{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway:                 gateway,
		PrecommitWatchdogBudget: 45 * time.Millisecond,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "synthetic-only", Home: "/synthetic/profile", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()
	for _, path := range []string{"/backend-api/godex/responses", "/backend-api/godex/responses/compact"} {
		response, err := http.Post(server.URL+path, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusGatewayTimeout ||
			!strings.Contains(string(body), "request timed out while waiting for upstream recovery before output started") {
			t.Fatalf("path %s status=%d body=%q want typed 504", path, response.StatusCode, body)
		}
	}
	if !gateway.cancelled.Load() || gateway.attempts.Load() != 2 {
		t.Fatalf("transport work remained active or was replayed: attempts=%d canceled=%t",
			gateway.attempts.Load(), gateway.cancelled.Load())
	}
}
