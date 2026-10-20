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

func TestProdex04383HTTPTurnMetadataNestedSessionIdentity(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, metadata, bodySession string
			want                        string
		}{
			{"nested", `{"client_metadata":{"session_id":"shadow-session"}}`, "", websocketQuotaOwnerB},
			{"direct", `{"session_id":"shadow-session"}`, "", websocketQuotaOwnerB},
			{"null-shadow", `{"session_id":null,"client_metadata":{"session_id":"shadow-session"}}`, "", websocketQuotaOwnerA},
			{"blank-shadow", `{"session_id":"  ","client_metadata":{"session_id":"shadow-session"}}`, "", websocketQuotaOwnerA},
			{"invalid-header-body-fallback", `{"session_id":null}`, `"session_id":"shadow-session",`, websocketQuotaOwnerB},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-http-metadata","object":"response","output":[]}`))}
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
					affinityKeys{session: "shadow-session"}, router.now()); err != nil {
					t.Fatal(err)
				}
				request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
					Header:         http.Header{"X-Codex-Turn-Metadata": {tc.metadata}},
					Body:           []byte(`{"model":"gpt-6",` + tc.bodySession + `"input":[{"role":"user","content":"independent"}]}`),
					QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != tc.want || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("HTTP metadata=%s selected=%q want=%q", tc.name, result.Result.AccountID, tc.want)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{session: "shadow-session"}, router.now())
				if err != nil || owner != websocketQuotaOwnerB {
					t.Fatalf("verified session owner changed: owner=%q err=%v", owner, err)
				}
			})
		}
	}
}
