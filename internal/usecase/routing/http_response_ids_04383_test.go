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

// Prodex's compiled response metadata planner records up to three distinct
// response IDs per JSON object, in nested, response_id, root-id order.
// An alternate valid ID must not lose continuation affinity on restart.
func TestProdex04383HTTPResponseAllIdentityAliasesRemainOwned(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, payload string
			want          []string
		}{
			{"triple", `{"response":{"id":"resp-inner"},"response_id":"resp-middle","object":"response","id":"resp-outer","output":[]}`, []string{"resp-inner", "resp-middle", "resp-outer"}},
			{"duplicate", `{"response":{"id":"resp-inner"},"response_id":"resp-inner","object":"response","id":"resp-outer","output":[]}`, []string{"resp-inner", "resp-outer"}},
			{"root-and-legacy", `{"response_id":"resp-middle","object":"response","id":"resp-outer","output":[]}`, []string{"resp-middle", "resp-outer"}},
			{"not-response-object", `{"response_id":"resp-middle","object":"event","id":"event-unrelated","output":[]}`, []string{"resp-middle"}},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK,
						Header: http.Header{"Content-Type": {"application/json"}},
						Body:   io.NopCloser(strings.NewReader(tc.payload))}
				}}
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
				exchange, err := router.Forward(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
					Header: make(http.Header), Body: []byte(`{"model":"gpt-6","input":[{"role":"user","content":"independent"}]}`),
					QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}})
				if err != nil {
					t.Fatal(err)
				}
				defer exchange.Close()
				if exchange.Result.AccountID != websocketQuotaOwnerA || exchange.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("unexpected initial response status/owner: %d %q", exchange.Result.Response.StatusCode, exchange.Result.AccountID)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for _, id := range tc.want {
					owner, err := store.owner(t.Context(), affinityKeys{previous: id}, router.now())
					if err != nil || owner != websocketQuotaOwnerA {
						t.Fatalf("response alias %q not verified on selected profile: owner=%q err=%v", id, owner, err)
					}
				}
				if tc.name == "not-response-object" {
					owner, err := store.owner(t.Context(), affinityKeys{previous: "event-unrelated"}, router.now())
					if err != nil || owner != "" {
						t.Fatalf("non-response event id bound as continuation: owner=%q err=%v", owner, err)
					}
				}
			})
		}
	}
}

// A precommitted SSE prelude can contain multiple metadata envelopes. The
// same IDs must be observable after the committed streaming body continues.
func TestProdex04383SSEBatchedResponseIdentityAliasesRemainOwned(t *testing.T) {
	const events = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-first\"},\"response_id\":\"resp-middle\",\"object\":\"response\",\"id\":\"resp-third\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-final\"},\"response_id\":\"resp-fifth\",\"object\":\"response\",\"id\":\"resp-sixth\"}\n\n"
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			config := Config{PreferredAccount: websocketQuotaOwnerA,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
			if durable {
				config.Bindings = routingrepository.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.Observe(t.Context(), websocketQuotaOwnerA, nil, []byte(events), true); err != nil {
				t.Fatal(err)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			for _, id := range []string{"resp-first", "resp-middle", "resp-third", "resp-final", "resp-fifth", "resp-sixth"} {
				owner, err := store.owner(t.Context(), affinityKeys{previous: id}, router.now())
				if err != nil || owner != websocketQuotaOwnerA {
					t.Fatalf("SSE ID %s lost owner: owner=%q err=%v", id, owner, err)
				}
			}
		})
	}
}
