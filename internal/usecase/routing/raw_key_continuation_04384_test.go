package routing

import (
	"context"
	"errors"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04384RawKeyContinuationFailsBeforeDispatch(t *testing.T) {
	assertRawKeyContinuationFailsBeforeDispatch(t, "deepseek")
}

func TestProdex04384GeminiRawKeyContinuationFailsBeforeDispatch(t *testing.T) {
	assertRawKeyContinuationFailsBeforeDispatch(t, "gemini")
}

func assertRawKeyContinuationFailsBeforeDispatch(t *testing.T, provider string) {
	t.Helper()
	for _, fixture := range []struct {
		name, previous, session, turn string
	}{
		{name: "response", previous: "resp-raw-key"},
		{name: "session", session: "session-raw-key"},
		{name: "turn", turn: "turn-raw-key"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			repository := routingrepo.NewStore(t.TempDir())
			profileHome := t.TempDir()
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{{
				status: http.StatusOK, contentType: "application/json",
				body: `{"object":"response","id":"resp-raw-key","output":[]}`,
			}}}
			if fixture.turn != "" {
				gateway.headers = []http.Header{{"X-Codex-Turn-State": {fixture.turn}}}
			}
			newRouter := func(id, endpoint string) *Router {
				t.Helper()
				router, err := NewRouter(Config{Gateway: gateway, Bindings: repository,
					Accounts: func(context.Context) ([]proxymodel.Account, error) {
						return []proxymodel.Account{{ID: id, Home: profileHome, Enabled: true, EphemeralAPIKey: true,
							Provider: proxymodel.Provider{Kind: provider, APIURL: endpoint}}}, nil
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				return router
			}
			const firstID = "aabbccddeeff00112233445566778899"
			first := newRouter(firstID, "http://127.0.0.1:1234/v1")
			request := proxymodel.Request{Header: make(http.Header),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			if fixture.session != "" {
				request.Header.Set("Session-Id", fixture.session)
			}
			exchange, err := first.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if err := exchange.Close(); err != nil {
				t.Fatal(err)
			}
			if fixture.previous != "" {
				request.Body = []byte(`{"previous_response_id":"` + fixture.previous + `"}`)
			}
			if fixture.turn != "" {
				request.Header.Set("X-Codex-Turn-State", fixture.turn)
			}
			for _, router := range []*Router{first,
				newRouter(firstID, "http://127.0.0.1:1234/v1"),
				newRouter("11223344556677889900aabbccddeeff", "http://127.0.0.1:1234/v1"),
				newRouter(firstID, "http://127.0.0.1:5678/v1"),
			} {
				exchange, err := router.Forward(t.Context(), request)
				if len(gateway.requests) != 1 {
					if exchange != nil {
						_ = exchange.Close()
					}
					t.Fatalf("bound continuation added %d upstream attempts", len(gateway.requests)-1)
				}
				if exchange != nil {
					_ = exchange.Close()
					t.Fatal("bound raw-key continuation reached the gateway")
				}
				var routeError *proxymodel.Error
				if !errors.As(err, &routeError) || routeError.StatusCode != http.StatusServiceUnavailable ||
					routeError.Message != "bound continuation provider is unavailable" {
					t.Fatalf("continuation rejection = %v", err)
				}
			}
			if len(gateway.requests) != 1 {
				t.Fatalf("continuation added upstream attempts: %v", gateway.requests)
			}
			for _, body := range []string{`{}`, `{"previous_response_id":"resp-unknown"}`} {
				gateway.replies = append(gateway.replies, responseRecoveryReply{status: http.StatusOK,
					contentType: "application/json", body: `{"id":"resp-fresh","output":[]}`})
				fresh, err := first.Forward(t.Context(), proxymodel.Request{Header: make(http.Header), Body: []byte(body),
					QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}})
				if err != nil {
					t.Fatalf("fresh or unknown continuation rejected: %v", err)
				}
				if err := fresh.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if len(gateway.requests) != 3 {
				t.Fatalf("fresh and unknown controls made %d attempts, want 3 total", len(gateway.requests))
			}
		})
	}
}
