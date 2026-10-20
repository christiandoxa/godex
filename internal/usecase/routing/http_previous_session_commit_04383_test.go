package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// The tagged 0.438.3 runtime remembers a successful HTTP Responses session
// even when a verified previous_response_id hard-pinned owner selection.
// A newly owned session and a conflicting existing session have deliberately
// different durable outcomes; neither may rotate the previous-response owner.
func TestProdex04383HTTPPreviousResponseCommitPersistsSessionOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, foreign := range []bool{false, true} {
			name := "memory/new-session"
			if foreign {
				name = "memory/foreign-session"
			}
			if durable {
				name = strings.Replace(name, "memory/", "durable/", 1)
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-http-hard-previous","object":"response","output":[]}`))}
				}}
				config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerB,
					Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
					affinityKeys{previous: "resp-main"}, router.now()); err != nil {
					t.Fatal(err)
				}
				if foreign {
					if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
						affinityKeys{session: "http-hard-session"}, router.now()); err != nil {
						t.Fatal(err)
					}
				}
				body := `{"model":"gpt-6","previous_response_id":"resp-main","session_id":"http-hard-session","input":[{"role":"user","content":"followup"}]}`
				request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
					Header: make(http.Header), Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != websocketQuotaOwnerA || result.Result.Failed || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("hard previous owner changed: account=%q failed=%t status=%d", result.Result.AccountID, result.Result.Failed, result.Result.Response.StatusCode)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				want := websocketQuotaOwnerA
				if foreign {
					want = routingentity.ConflictAccountID
				}
				owner, err := store.owner(t.Context(), affinityKeys{session: "http-hard-session"}, router.now())
				if foreign {
					if err == nil || owner != "" {
						t.Fatalf("foreign session did not conflict: owner=%q err=%v", owner, err)
					}
				} else if err != nil || owner != want {
					t.Fatalf("new direct HTTP session owner=%q want=%q err=%v", owner, want, err)
				}
				key := affinityDigest("session", "http-hard-session")
				stored := ""
				if durable {
					bindings, err := config.Bindings.Load(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					for _, binding := range bindings {
						if binding.Key == key {
							stored = binding.AccountID
						}
					}
				} else {
					store.mu.Lock()
					stored = store.values[key].accountID
					store.mu.Unlock()
				}
				if stored != want {
					t.Fatalf("durable session owner=%q want=%q", stored, want)
				}

				prev, err := store.owner(t.Context(), affinityKeys{previous: "resp-main"}, router.now())
				if err != nil || prev != websocketQuotaOwnerA {
					t.Fatalf("verified previous owner changed: owner=%q err=%v", prev, err)
				}
			})
		}
	}

}

// Tagged 0.438.3 persists direct session observations after a hard turn-state
// owner wins HTTP Responses selection, independently of the session affinity.
func TestProdex04383HTTPTurnStateCommitPersistsSessionOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, foreign := range []bool{false, true} {
			name := "memory/new-session"
			if foreign {
				name = "memory/foreign-session"
			}
			if durable {
				name = strings.Replace(name, "memory/", "durable/", 1)
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-http-hard-previous","object":"response","output":[]}`))}
				}}
				config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerB,
					Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
					affinityKeys{turn: "turn-a"}, router.now()); err != nil {
					t.Fatal(err)
				}
				if foreign {
					if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
						affinityKeys{session: "http-hard-session"}, router.now()); err != nil {
						t.Fatal(err)
					}
				}
				body := `{"model":"gpt-6","x-codex-turn-state":"turn-a","session_id":"http-hard-session","input":[{"role":"user","content":"followup"}]}`
				request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
					Header: make(http.Header), Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != websocketQuotaOwnerA || result.Result.Failed || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("hard turn owner changed: account=%q failed=%t status=%d", result.Result.AccountID, result.Result.Failed, result.Result.Response.StatusCode)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				want := websocketQuotaOwnerA
				if foreign {
					want = routingentity.ConflictAccountID
				}
				owner, err := store.owner(t.Context(), affinityKeys{session: "http-hard-session"}, router.now())
				if foreign {
					if err == nil || owner != "" {
						t.Fatalf("foreign session did not conflict: owner=%q err=%v", owner, err)
					}
				} else if err != nil || owner != want {
					t.Fatalf("new direct HTTP session owner=%q want=%q err=%v", owner, want, err)
				}
				key := affinityDigest("session", "http-hard-session")
				stored := ""
				if durable {
					bindings, err := config.Bindings.Load(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					for _, binding := range bindings {
						if binding.Key == key {
							stored = binding.AccountID
						}
					}
				} else {
					store.mu.Lock()
					stored = store.values[key].accountID
					store.mu.Unlock()
				}
				if stored != want {
					t.Fatalf("durable session owner=%q want=%q", stored, want)
				}

				turn, err := store.owner(t.Context(), affinityKeys{turn: "turn-a"}, router.now())
				if err != nil || turn != websocketQuotaOwnerA {
					t.Fatalf("verified turn owner changed: owner=%q err=%v", turn, err)
				}
			})
		}
	}

}
