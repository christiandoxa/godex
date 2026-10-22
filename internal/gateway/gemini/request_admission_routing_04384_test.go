package gemini_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/gemini"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestProdex04384GeminiRequestAdmissionDoesNotRetryModelOrCredential(t *testing.T) {
	for _, fixture := range []struct {
		body, message string
		status        int
	}{
		{"null", "", 500},
		{"{bad", "upstream request failed", 502},
		{`{"input":"synthetic","candidate_count":2}`, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1", 400},
		{`{"input":"synthetic","candidateCount":2}`, "invalid_candidate_count: Gemini request field `candidateCount` must be omitted, null, or 1", 400},
		{`{"input":"synthetic","candidate_count":1,"candidateCount":2}`, "invalid_candidate_count: Gemini request fields `candidate_count` and `candidateCount` conflict", 400},
		{`{"input":7,"candidate_count":2,"tools":false}`, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1", 400},
	} {
		t.Run(fixture.body, func(t *testing.T) {
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
			defer router.Close()
			exchange, err := router.Forward(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Header: make(http.Header), Body: []byte(fixture.body)})
			if exchange != nil {
				defer exchange.Close()
			}
			if attempts.Load() != 0 || waits.Load() != 0 {
				t.Fatalf("invalid JSON admission entered upstream or wait: attempts=%d waits=%d", attempts.Load(), waits.Load())
			}
			if fixture.status != 500 {
				var failure *proxymodel.Error
				if exchange != nil || !errors.As(err, &failure) || failure.StatusCode != fixture.status || failure.Message != fixture.message {
					t.Fatalf("invalid request lost terminal preparation failure: %v", err)
				}
				return
			}
			if err != nil || exchange == nil {
				t.Fatalf("nonobject root lost empty terminal response: %v", err)
			}
			response := exchange.Result.Response
			payload, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != 500 || response.Header.Get("Content-Type") != "" || len(payload)+len(exchange.Result.Prefix) != 0 {
				t.Fatal("nonobject root did not retain exact empty 500")
			}
		})
	}
}
