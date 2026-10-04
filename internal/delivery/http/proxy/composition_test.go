package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/codex"
	"github.com/christiandoxa/godex/internal/gateway/openai"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type RuntimeAccount = proxymodel.Account

// Test composition deliberately constructs adapters outside production delivery.
type ProxyConfig struct {
	ListenAddr, UpstreamURL, PreferredAccount string
	Accounts                                  func(context.Context) ([]RuntimeAccount, error)
	Client                                    *http.Client
	Now                                       func() time.Time
	Wait                                      func(context.Context, time.Duration) error
	MaxRequestBytes, MaxInspectBytes          int64
	Activity                                  activityRecorder
	Bindings                                  *routingrepo.Store
}

func newProxyForTest(config ProxyConfig) (*Proxy, error) {
	transport, err := openai.NewTransport(config.UpstreamURL, config.Client, codex.NewCodexProcess("", codex.Terminal{}))
	if err != nil {
		return nil, err
	}
	routingConfig := routingusecase.Config{
		Gateway: transport, Accounts: config.Accounts, PreferredAccount: config.PreferredAccount,
		Now: config.Now, Wait: config.Wait, MaxInspectBytes: config.MaxInspectBytes,
	}
	if config.Bindings != nil {
		routingConfig.Bindings = config.Bindings
	}
	router, err := routingusecase.NewRouter(routingConfig)
	if err != nil {
		return nil, err
	}
	return NewProxy(Config{Router: router, Activity: config.Activity, ListenAddr: config.ListenAddr, MaxRequestBytes: config.MaxRequestBytes, MaxInspectBytes: config.MaxInspectBytes})
}

func newParityProxyWithConfig(t *testing.T, config ProxyConfig) *httptest.Server {
	t.Helper()
	proxy, err := newProxyForTest(config)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	t.Cleanup(server.Close)
	return server
}
