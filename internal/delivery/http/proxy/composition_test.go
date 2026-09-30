package proxy

import (
	"context"
	"net/http"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/codex"
	"github.com/christiandoxa/godex/internal/gateway/openai"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type RuntimeAccount = proxymodel.Account

// Test composition deliberately constructs adapters outside production delivery.
type ProxyConfig struct {
	ListenAddr, UpstreamURL, PreferredAccount string
	Accounts                                  func(context.Context) ([]RuntimeAccount, error)
	Client                                    *http.Client
	Now                                       func() time.Time
	MaxRequestBytes, MaxInspectBytes          int64
}

func newProxyForTest(config ProxyConfig) (*Proxy, error) {
	transport, err := openai.NewTransport(config.UpstreamURL, config.Client, codex.NewCodexProcess("", codex.Terminal{}))
	if err != nil {
		return nil, err
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{Gateway: transport, Accounts: config.Accounts, PreferredAccount: config.PreferredAccount, Now: config.Now, MaxInspectBytes: config.MaxInspectBytes})
	if err != nil {
		return nil, err
	}
	return NewProxy(Config{Router: router, ListenAddr: config.ListenAddr, MaxRequestBytes: config.MaxRequestBytes, MaxInspectBytes: config.MaxInspectBytes})
}
