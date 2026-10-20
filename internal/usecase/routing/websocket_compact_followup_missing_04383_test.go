package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// Prodex 0.438.3's strict compact followup cannot move across accounts even
// when a bound turn-only request receives a missing-response error before
// commitment. The original owner and opaque request must remain intact.
func TestProdex04383CompactFollowupMissingResponseCannotRotate(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("missing")), WebSocketFrames: true,
					PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found"}},
				websocketCommittedTestResponse("unexpected-foreign-replay"),
			}}
			config := Config{Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) {
				return websocketQuotaAccounts(), nil
			}}
			if durable {
				config.Bindings = routingrepository.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
				affinityKeys{turn: "compact-turn", session: "compact-session"}, router.now()); err != nil {
				t.Fatal(err)
			}
			request := websocketDispatchRequest(`{"type":"response.create","session_id":"compact-session","client_metadata":{"x-codex-turn-state":"compact-turn"},"input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"followup"}]}`, 843307)
			request.Header.Set("x-codex-turn-state", "compact-turn")
			request.QuotaSelection.RouteKind = quotamodel.RouteKindCompact
			result, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if result.Result.AccountID != websocketQuotaOwnerA || len(gateway.accounts) != 1 || gateway.accounts[0] != websocketQuotaOwnerA {
				t.Fatalf("strict compact followup crossed owner after upstream missing response: owner=%q attempts=%v", result.Result.AccountID, gateway.accounts)
			}
			if result.Result.Response == nil || result.Result.Response.PrecommitFailure == nil ||
				result.Result.Response.PrecommitFailure.Code != "previous_response_not_found" {
				t.Fatalf("compact followup lost original precommit failure: %+v", result.Result.Response)
			}
			if len(gateway.requests) != 1 || string(gateway.requests[0].Body) != string(request.Body) {
				t.Fatalf("compact followup lost opaque input: attempts=%d", len(gateway.requests))
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			for kind, key := range map[string]string{"turn": "compact-turn", "session": "compact-session"} {
				keys := affinityKeys{turn: key}
				if kind == "session" {
					keys = affinityKeys{session: key}
				}
				owner, err := store.owner(t.Context(), keys, router.now())
				if err != nil || owner != websocketQuotaOwnerA {
					t.Fatalf("strict compact %s binding changed: owner=%q err=%v", kind, owner, err)
				}
			}
		})
	}
}
