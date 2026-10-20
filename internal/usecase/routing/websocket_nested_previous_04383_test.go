package routing

import (
	"context"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// The tagged 0.438.3 WebSocket semantic planner uses only the direct
// previous_response_id JSON field. A nested response object is opaque input,
// not evidence permitting another account to resume a previous response.
func TestProdex04383WebSocketNestedPreviousCannotClaimHardOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, members, want string
			pinned              bool
		}{
			{"nested-only", `"response":{"previous_response_id":"resp-foreign"},`, websocketQuotaOwnerA, false},
			{"nested-direct-null", `"previous_response_id":null,"response":{"previous_response_id":"resp-foreign"},`, websocketQuotaOwnerA, false},
			{"nested-direct-blank", `"previous_response_id":"  ","response":{"previous_response_id":"resp-foreign"},`, websocketQuotaOwnerA, false},
			{"direct-foreign", `"previous_response_id":"resp-foreign","response":{"previous_response_id":"resp-main"},`, websocketQuotaOwnerB, true},
			{"direct-main", `"previous_response_id":"resp-main","response":{"previous_response_id":"resp-foreign"},`, websocketQuotaOwnerA, true},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{websocketCommittedTestResponse("resp-nested-result")}}
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
				for _, owner := range []struct{ id, previous string }{
					{websocketQuotaOwnerB, "resp-foreign"}, {websocketQuotaOwnerA, "resp-main"},
				} {
					if err := router.affinity.remember(t.Context(), owner.id,
						affinityKeys{previous: owner.previous}, router.now()); err != nil {
						t.Fatal(err)
					}
				}
				body := `{"type":"response.create","model":"gpt-6",` + tc.members + `"input":[{"role":"user","content":"independent history"}]}`
				request := websocketDispatchRequest(body, 843389)
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != tc.want || len(gateway.accounts) != 1 || gateway.accounts[0] != tc.want {
					t.Fatalf("WebSocket nested previous selected=%q want=%q requests=%v", result.Result.AccountID, tc.want, gateway.accounts)
				}
				if len(gateway.requests) != 1 || string(gateway.requests[0].Body) != body ||
					gateway.requests[0].WebSocketPolicy.RequestPreviousResponse != tc.pinned ||
					strings.TrimSpace(gateway.requests[0].Header.Get("x-codex-turn-state")) != "" {
					t.Fatalf("opaque nested previous altered request/policy: %+v", gateway.requests)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for _, check := range []struct{ key, owner string }{
					{"resp-foreign", websocketQuotaOwnerB}, {"resp-main", websocketQuotaOwnerA},
					{"resp-nested-result", tc.want},
				} {
					owner, err := store.owner(t.Context(), affinityKeys{previous: check.key}, router.now())
					if err != nil || owner != check.owner {
						t.Fatalf("response %s owner=%q want=%q err=%v", check.key, owner, check.owner, err)
					}
				}
			})
		}
	}
}
