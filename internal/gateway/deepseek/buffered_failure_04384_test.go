package deepseek_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/deepseek"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestProdex04384BufferedTranslationFailureDoesNotRotateCredentials(t *testing.T) {
	for _, fixture := range []struct {
		name, body                    string
		status, wantStatus, wantCalls int
	}{
		{"malformed", "{bad", 200, 502, 1},
		{"oversized", strings.Repeat(" ", (4<<20)+1), 200, 502, 1},
		{"upstream_failure_rotates", `{"error":{"code":"internal_server_error"}}`, 502, 200, 2},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := int(attempts.Add(1))
				if attempt > fixture.wantCalls {
					cancel()
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if attempt == 1 {
					w.WriteHeader(fixture.status)
					_, _ = io.WriteString(w, fixture.body)
					return
				}
				_, _ = io.WriteString(w, `{"id":"resp-control","created":1,"choices":[{"message":{"content":"ok"}}]}`)
			}))
			defer server.Close()
			pool, err := deepseek.NewRuntimePool(server.URL, []proxymodel.ProviderCredential{
				{ID: "primary", Secret: "synthetic-primary"}, {ID: "secondary", Secret: "synthetic-secondary"},
			}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			router, err := routing.NewRouter(routing.Config{Gateway: pool, PreferredAccount: "primary",
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{
						{ID: "primary", Home: "/synthetic/primary", Enabled: true, Provider: proxymodel.Provider{Kind: "deepseek"}},
						{ID: "secondary", Home: "/synthetic/secondary", Enabled: true, Provider: proxymodel.Provider{Kind: "deepseek"}},
					}, nil
				}})
			if err != nil {
				t.Fatal(err)
			}
			exchange, err := router.Forward(ctx, proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Header: make(http.Header), Body: []byte(`{"model":"pro","input":"hello"}`)})
			if err != nil {
				t.Fatalf("terminal processing failure entered credential retry: attempts=%d error=%v", attempts.Load(), err)
			}
			defer exchange.Close()
			response := exchange.Result.Response
			body, err := io.ReadAll(response.Body)
			body = append(append([]byte(nil), exchange.Result.Prefix...), body...)
			if err != nil || response.StatusCode != fixture.wantStatus || int(attempts.Load()) != fixture.wantCalls {
				t.Fatalf("processing failure status/attempts = %d/%d, error=%v", response.StatusCode, attempts.Load(), err)
			}
			if fixture.wantStatus == 502 && (!exchange.Result.Failed || string(body) != "provider response could not be processed" ||
				response.Header.Get("Content-Type") != "text/plain; charset=utf-8") {
				t.Fatal("terminal processing failure changed its original error response")
			}
		})
	}
}
