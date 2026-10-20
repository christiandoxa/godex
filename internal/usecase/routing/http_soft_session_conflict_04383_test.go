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

// The tagged 0.438.3 Responses route excludes conflicting soft session owners
// from selection while preserving the conflict marker. Hard continuation
// conflicts remain fail-closed and must not inherit this permissive rule.
func TestProdex04383HTTPResponsesConflictedSoftSessionSelectsFresh(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
				return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{"id":"resp-http-conflict","object":"response","output":[]}`))}
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
			for _, owner := range []string{websocketQuotaOwnerB, websocketQuotaOwnerA} {
				if err := router.affinity.rememberVerified(t.Context(), owner,
					affinityKeys{session: "http-conflict"}, router.now()); err != nil {
					t.Fatal(err)
				}
			}
			if owner, err := router.affinity.owner(t.Context(), affinityKeys{session: "http-conflict"}, router.now()); err == nil || owner != "" {
				t.Fatalf("failed to seed verified session conflict: owner=%q err=%v", owner, err)
			}
			request := proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/codex/responses", Header: make(http.Header),
				Body:           []byte(`{"model":"gpt-6","session_id":"http-conflict","input":[{"role":"user","content":"hello"}]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			result, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatalf("conflicted soft HTTP session failed before upstream: %v", err)
			}
			defer result.Close()
			if result.Result.AccountID != websocketQuotaOwnerA || result.Result.Response.StatusCode != http.StatusOK {
				t.Fatalf("conflicted HTTP session routed unexpectedly: owner=%q status=%d", result.Result.AccountID, result.Result.Response.StatusCode)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			key := affinityDigest("session", "http-conflict")
			want := routingentity.ConflictAccountID
			if owner, err := store.owner(t.Context(), affinityKeys{session: "http-conflict"}, router.now()); err == nil || owner != "" {
				t.Fatalf("committed HTTP session erased conflict: owner=%q err=%v", owner, err)
			}
			if durable {
				all, err := config.Bindings.Load(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, item := range all {
					if item.Key == key {
						found = true
						if item.AccountID != want {
							t.Fatalf("persisted account=%q want conflict", item.AccountID)
						}
					}
				}
				if !found {
					t.Fatal("missing durable conflict marker")
				}
			} else {
				store.mu.Lock()
				owner := store.values[key].accountID
				store.mu.Unlock()
				if owner != want {
					t.Fatalf("conflict marker=%q", owner)
				}
			}
		})
	}
}

// The new fallback is exclusive to the verified conflict marker: an ordinary
// owned soft HTTP Responses session must still prefer its original account
// over an unrelated active/preferred account.
func TestProdex04383HTTPResponsesHealthySessionRetainsSoftOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
				return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{"id":"resp-http-owned","object":"response","output":[]}`))}
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
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
				affinityKeys{session: "http-owned"}, router.now()); err != nil {
				t.Fatal(err)
			}
			request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
				Header: make(http.Header), Body: []byte(`{"model":"gpt-6","session_id":"http-owned","input":[]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			result, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if result.Result.AccountID != websocketQuotaOwnerB || result.Result.Response.StatusCode != http.StatusOK {
				t.Fatalf("normal session owner lost: owner=%q status=%d", result.Result.AccountID, result.Result.Response.StatusCode)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			owner, err := store.owner(t.Context(), affinityKeys{session: "http-owned"}, router.now())
			if err != nil || owner != websocketQuotaOwnerB {
				t.Fatalf("normal session durability changed: owner=%q err=%v", owner, err)
			}
		})
	}
}
