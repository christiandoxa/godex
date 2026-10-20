package routing

import (
	"context"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// The canonical 0.438.3 WebSocket semantic plan reads the precise
// x-codex-turn-state JSON member or client_metadata key; turn_state is not
// a request alias, even though it is valid in *response* metadata.
func TestProdex04383WebSocketLegacyTurnFieldCannotCreateHardOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, fields, want string
			valid              bool
		}{
			{"legacy-top", `"turn_state":"turn-foreign",`, websocketQuotaOwnerA, false},
			{"legacy-nested", `"client_metadata":{"turn_state":"turn-foreign"},`, websocketQuotaOwnerA, false},
			{"legacy-both", `"turn_state":"turn-foreign","client_metadata":{"turn_state":"turn-foreign"},`, websocketQuotaOwnerA, false},
			{"canonical-direct", `"x-codex-turn-state":"turn-foreign",`, websocketQuotaOwnerB, true},
			{"canonical-nested", `"client_metadata":{"x-codex-turn-state":"turn-foreign"},`, websocketQuotaOwnerB, true},
			{"direct-null-shadows", `"x-codex-turn-state":null,"client_metadata":{"x-codex-turn-state":"turn-foreign"},`, websocketQuotaOwnerA, false},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{websocketCommittedTestResponse("resp-turn-alias")}}
				config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
					Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
					affinityKeys{turn: "turn-foreign"}, router.now()); err != nil {
					t.Fatal(err)
				}
				body := `{"type":"response.create","model":"gpt-6",` + tc.fields + `"input":[{"role":"user","content":"independent"}]}`
				request := websocketDispatchRequest(body, 843387)
				response, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Close()
				if response.Result.AccountID != tc.want || len(gateway.accounts) != 1 || gateway.accounts[0] != tc.want {
					t.Fatalf("legacy turn alias routed across owner: got=%q want=%q upstream=%v", response.Result.AccountID, tc.want, gateway.accounts)
				}
				if len(gateway.requests) != 1 || gateway.requests[0].WebSocketPolicy.RequestTurnState != tc.valid ||
					string(gateway.requests[0].Body) != body {
					t.Fatalf("legacy turn alias rewrote request or granted wrong turn owner: request=%+v", gateway.requests)
				}
				if !tc.valid && strings.TrimSpace(gateway.requests[0].Header.Get("x-codex-turn-state")) != "" {
					t.Fatalf("unsupported JSON turn alias became an upstream handshake header: %+v", gateway.requests[0].Header)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{turn: "turn-foreign"}, router.now())
				if err != nil || owner != websocketQuotaOwnerB {
					t.Fatalf("foreign turn owner changed: owner=%q err=%v", owner, err)
				}
			})
		}
	}
}
