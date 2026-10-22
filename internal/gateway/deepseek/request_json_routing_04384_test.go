package deepseek_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/deepseek"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestProdex04384DeepSeekJSONAdmissionDoesNotRetryModelOrCredential(t *testing.T) {
	for _, body := range []string{"null", `{"input":`} {
		t.Run(body, func(t *testing.T) {
			var attempts, waits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			pool, err := deepseek.NewRuntimePool(server.URL, []proxymodel.ProviderCredential{
				{ID: "primary", Secret: "synthetic-primary"}, {ID: "secondary", Secret: "synthetic-secondary"},
			}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			router, err := routing.NewRouter(routing.Config{Gateway: pool,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{
						{ID: "primary", Home: "/synthetic/primary", Enabled: true, Provider: proxymodel.Provider{Kind: "deepseek"}},
						{ID: "secondary", Home: "/synthetic/secondary", Enabled: true, Provider: proxymodel.Provider{Kind: "deepseek"}},
					}, nil
				}, Wait: func(context.Context, time.Duration) error {
					waits.Add(1)
					return errors.New("unexpected retry wait")
				}})
			if err != nil {
				t.Fatal(err)
			}
			exchange, err := router.Forward(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Header: make(http.Header), Body: []byte(body)})
			if exchange != nil {
				defer exchange.Close()
			}
			if attempts.Load() != 0 || waits.Load() != 0 {
				t.Fatalf("invalid JSON entered upstream or wait: attempts=%d waits=%d", attempts.Load(), waits.Load())
			}
			if body != "null" {
				var failure *proxymodel.Error
				if exchange != nil || !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
					t.Fatalf("malformed JSON lost terminal preparation failure: %v", err)
				}
				return
			}
			if err != nil || exchange == nil {
				t.Fatalf("nonobject root lost empty terminal response: %v", err)
			}
			response := exchange.Result.Response
			payload, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != 500 || len(response.Header["Content-Type"]) != 0 || len(payload)+len(exchange.Result.Prefix) != 0 {
				t.Fatal("nonobject root did not retain exact empty 500")
			}
		})
	}
}
