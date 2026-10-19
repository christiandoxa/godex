package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04383HTTPResponsesOwnedTurnTransportFailureDoesNotReplay(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "durable"}[persisted], func(t *testing.T) {
			gateway := &prodex04356Gateway{steps: []prodex04356Step{
				{account: websocketQuotaOwnerA, err: &url.Error{Op: "Post", Err: io.EOF}},
				{account: websocketQuotaOwnerB, status: 200, body: `{"id":"must-not-replay"}`},
			}}
			config := Config{Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
			if persisted {
				config.Bindings = routingrepo.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			keys := affinityKeys{turn: "turn-http", session: "session-http", previous: "unrelated"}
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, keys, router.now()); err != nil {
				t.Fatal(err)
			}
			request := proxymodel.Request{
				Method: http.MethodPost, Path: "/responses", Header: http.Header{"X-Codex-Turn-State": {"turn-http"}},
				Body:           []byte(`{"session_id":"session-http","client_metadata":{"x-codex-turn-state":"turn-http"},"input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			}
			if !requestHasReconstructableFullHistory(request) {
				t.Fatal("fixture must prove that full history does not authorize this transport replay")
			}
			exchange, err := router.Forward(t.Context(), request)
			if exchange != nil {
				defer exchange.Close()
			}
			var failure *proxymodel.Error
			if !errors.As(err, &failure) || failure.StatusCode != http.StatusServiceUnavailable || failure.Message != upstreamTransportFailureMessage {
				t.Fatalf("owned HTTP turn failure=%v, want exact local transport 503", err)
			}
			if strings.Join(gateway.calls, ",") != websocketQuotaOwnerA {
				t.Fatalf("owned HTTP turn was replayed: %v", gateway.calls)
			}
			store := router.affinity
			if persisted {
				store = newAffinityStore()
				store.repository = config.Bindings
				if err := store.loadContinuationStatuses(t.Context(), router.now()); err != nil {
					t.Fatal(err)
				}
			}
			for _, retained := range []affinityKeys{{turn: keys.turn}, {session: keys.session}, {previous: keys.previous}} {
				owner, err := store.owner(t.Context(), retained, router.now())
				if err != nil || owner != websocketQuotaOwnerA {
					t.Fatalf("failed HTTP transport changed ownership: owner=%q err=%v", owner, err)
				}
			}
		})
	}
}
