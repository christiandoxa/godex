package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type requestTurnSSEGateway04383 struct {
	body  string
	calls atomic.Int32
}

func (g *requestTurnSSEGateway04383) Execute(_ context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	if g.calls.Add(1) == 1 {
		return &proxymodel.Response{StatusCode: http.StatusOK, Header: http.Header{
			"Content-Type": {"application/json"}, "X-Codex-Turn-State": {"state-request"},
		}, Body: io.NopCloser(strings.NewReader(`{"object":"response","id":"resp-warmup-turn"}`))}, nil
	}
	boundary := strings.Index(g.body, "\n\n") + 2
	return &proxymodel.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
		// Force the first output-delta event across the precommit boundary.
		// Later events must be handled by the committed stream observer,
		// just as the tagged executable fixture flushes and delays them.
		Body: io.NopCloser(io.MultiReader(strings.NewReader(g.body[:boundary]), strings.NewReader(g.body[boundary:])))}, nil
}

func TestProdex04383RequestTurnInheritsToCommittedSSEWithoutUpstreamTurn(t *testing.T) {
	for _, tc := range []struct {
		name, firstBody, expectedFirst, expectedSecond string
	}{
		{"no-response-turn", `{"type":"response.output_text.delta","delta":"committed","response":{"id":"resp-request-prefix"}}`, "state-request", "state-request"},
		// The tagged precommit planner records all lookahead response IDs
		// with the request override before the later SSE tap can change
		// its active turn state from the event's own token.
		{"conflicting-first-event", `{"type":"response.output_text.delta","delta":"committed","response":{"id":"resp-request-prefix","turn_state":"state-event"}}`, "state-request", "state-event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const account = websocketHardOwner04379
			prefix := "data: " + tc.firstBody + "\n\n"
			terminal := "data: " + `{"type":"response.completed","response":{"id":"resp-request-terminal"}}` + "\n\n"
			home := t.TempDir()
			if err := os.Chmod(home, 0o700); err != nil {
				t.Fatal(err)
			}
			store := routingrepo.NewStore(t.TempDir())
			router, err := routingusecase.NewRouter(routingusecase.Config{
				Gateway: &requestTurnSSEGateway04383{body: prefix + terminal}, Bindings: store, PreferredAccount: account,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{ID: account, Home: home, Enabled: true}}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			warmup, err := router.Forward(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header),
				Body:           []byte(`{"model":"gpt-6","input":[]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			})
			if err != nil {
				t.Fatal(err)
			}
			_ = warmup.Close()
			req := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
				Header:         http.Header{"X-Codex-Turn-State": {"state-request"}},
				Body:           []byte(`{"model":"gpt-6","stream":true,"input":[{"role":"user","content":"new turn"}]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			exchange, err := router.Forward(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			output := httptest.NewRecorder()
			proxy := &Proxy{router: router, maxInspect: 64 << 10}
			proxy.forwardResponse(t.Context(), output, exchange.Result.Response, exchange.Result.Prefix, exchange.Result.AccountID, &requestLifecycle{}, exchange.Result.ProviderKind)
			if output.Body.String() != prefix+terminal {
				t.Fatalf("stream bytes changed: %q", output.Body.String())
			}
			for _, check := range []struct{ id, turn string }{
				{"resp-request-prefix", tc.expectedFirst},
				{"resp-request-terminal", tc.expectedSecond},
			} {
				hash := sha256.Sum256([]byte("previous:" + check.id))
				key := hex.EncodeToString(hash[:])
				turn, _, err := store.LoadResponseTurnState(t.Context(), home, key, time.Now())
				if err != nil || turn != check.turn {
					t.Fatalf("response=%s turn=%q want=%q err=%v", check.id, turn, check.turn, err)
				}
			}
		})
	}
}
