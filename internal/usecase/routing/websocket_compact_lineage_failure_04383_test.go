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

// The official 0.438.3 executable permits one full-history precommit
// transport replay from a Compact turn alias to an authenticated Ready
// alternate, with both stale turn-state carriers removed. The old alias
// remains on its original owner rather than being rebound to the alternate.
func TestProdex04383WebSocketCompactLineageTransportReplaysScrubbedFullHistory(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				{StatusCode: http.StatusBadGateway, Header: make(http.Header),
					Body:            io.NopCloser(strings.NewReader("upstream closed")),
					WebSocketFrames: true, WebSocketTurnState: "compact-turn",
					PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true}},
				websocketCommittedTestResponse("resp-compact-replayed"),
			}}
			config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil },
			}
			if durable {
				config.Bindings = routingrepository.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
				affinityKeys{turn: "__compact_turn_state__:compact-turn"}, router.now()); err != nil {
				t.Fatal(err)
			}
			body := `{"type":"response.create","model":"gpt-6","x-codex-turn-state":"compact-turn","input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"next"}]}`
			request := websocketDispatchRequest(body, 843326)
			request.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
			result, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if len(gateway.accounts) != 2 || gateway.accounts[0] != websocketQuotaOwnerB ||
				gateway.accounts[1] != websocketQuotaOwnerA || result.Result.AccountID != websocketQuotaOwnerA ||
				result.Result.Failed {
				t.Fatalf("tagged Compact full-history transport handoff changed: owner=%q failed=%t attempts=%v", result.Result.AccountID, result.Result.Failed, gateway.accounts)
			}
			if len(gateway.requests) != 2 || string(gateway.requests[0].Body) != body ||
				gateway.requests[0].Header.Get("x-codex-turn-state") != "compact-turn" ||
				gateway.requests[1].Header.Get("x-codex-turn-state") != "" ||
				strings.Contains(string(gateway.requests[1].Body), `"x-codex-turn-state"`) ||
				!strings.Contains(string(gateway.requests[1].Body), `"content":"next"`) {
				t.Fatal("full-history handoff retained stale token or lost the opaque request")
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			for _, tc := range []struct {
				name string
				keys affinityKeys
				want string
			}{
				{"Compact alias", affinityKeys{turn: "__compact_turn_state__:compact-turn"}, websocketQuotaOwnerB},
				{"raw turn", affinityKeys{turn: "compact-turn"}, ""},
				{"recovered response", affinityKeys{previous: "resp-compact-replayed"}, websocketQuotaOwnerA},
			} {
				owner, err := store.owner(t.Context(), tc.keys, router.now())
				if err != nil || owner != tc.want {
					t.Fatalf("%s owner=%q want=%q err=%v", tc.name, owner, tc.want, err)
				}
			}
		})
	}
}
