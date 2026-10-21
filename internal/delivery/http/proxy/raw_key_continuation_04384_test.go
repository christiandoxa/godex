package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestProdex04384RawKeyContinuationHTTPRejectionSurvivesRestart(t *testing.T) {
	store := routingrepo.NewStore(t.TempDir())
	profileHome := t.TempDir()
	var calls atomic.Int32
	newHandler := func() *Proxy {
		t.Helper()
		router, err := routingusecase.NewRouter(routingusecase.Config{Bindings: store,
			Gateway: gatewayFunc(func(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
				calls.Add(1)
				return &proxymodel.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{"object":"response","id":"resp-raw-key","output":[]}`))}, nil
			}),
			Accounts: func(context.Context) ([]proxymodel.Account, error) {
				return []proxymodel.Account{{ID: strings.Repeat("a", 32), Home: profileHome, Enabled: true,
					EphemeralAPIKey: true, Provider: proxymodel.Provider{Kind: "deepseek"}}}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.Close)
		handler, err := NewProxy(Config{Router: router})
		if err != nil {
			t.Fatal(err)
		}
		return handler
	}
	first := newHandler()
	seed := httptest.NewRecorder()
	first.ServeHTTP(seed, httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"input":"fresh"}`)))
	if seed.Code != http.StatusOK {
		t.Fatalf("seed status = %d: %s", seed.Code, seed.Body.String())
	}
	for _, handler := range []*Proxy{first, newHandler()} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"previous_response_id":"resp-raw-key"}`)))
		if response.Code != http.StatusServiceUnavailable || response.Header().Get("Content-Type") != "application/json" ||
			response.Body.String() != `{"error":{"code":"bound_continuation_unavailable","message":"bound continuation provider is unavailable"}}` {
			t.Fatalf("raw-key continuation HTTP rejection = %d %s", response.Code, response.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("continuation reached upstream: %d calls", calls.Load())
	}
}
