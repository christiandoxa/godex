package gemini_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/gemini"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestProdex04384GeminiInvalidUserIDDoesNotRetryModelOrCredential(t *testing.T) {
	var attempts, waits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	pool, err := gemini.NewRuntimePool(server.URL, []proxymodel.ProviderCredential{
		{ID: "primary", Secret: "synthetic-primary"}, {ID: "secondary", Secret: "synthetic-secondary"},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	router, err := routing.NewRouter(routing.Config{Gateway: pool,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "primary", Home: "/synthetic/primary", Enabled: true, Provider: proxymodel.Provider{Kind: "gemini"}},
				{ID: "secondary", Home: "/synthetic/secondary", Enabled: true, Provider: proxymodel.Provider{Kind: "gemini"}},
			}, nil
		}, Wait: func(context.Context, time.Duration) error {
			waits.Add(1)
			return errors.New("unexpected retry wait")
		}})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{Method: http.MethodPost,
		Path: "/backend-api/godex/responses", Header: make(http.Header),
		Body: []byte(`{"model":"gemini-3.1-pro-preview","input":"hello","user_id":7}`)})
	if exchange != nil {
		_ = exchange.Close()
	}
	var failure *proxymodel.Error
	if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" ||
		exchange != nil || attempts.Load() != 0 || waits.Load() != 0 {
		t.Fatalf("local user ID failure entered upstream or retry: attempts=%d waits=%d error=%v", attempts.Load(), waits.Load(), err)
	}
}
