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

// Prodex 0.438.3's runtime_request_semantic_plan chooses JSON members by
// presence, not by a valid string value. A malformed direct session ID must
// shadow nested client_metadata and never route on another account's session.
func TestProdex04383HTTPDirectSessionShadowsNestedMetadata(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, direct string
			want         string
		}{
			{"null", `null`, websocketQuotaOwnerA},
			{"empty", `""`, websocketQuotaOwnerA},
			{"blank", `"  "`, websocketQuotaOwnerA},
			{"boolean", `false`, websocketQuotaOwnerA},
			{"number", `123`, websocketQuotaOwnerA},
			{"object", `{"session":"wrong"}`, websocketQuotaOwnerA},
			{"missing", ``, websocketQuotaOwnerB},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-http-shadow","object":"response","output":[]}`))}
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
					affinityKeys{session: "nested-owned-session"}, router.now()); err != nil {
					t.Fatal(err)
				}
				directField := ""
				if tc.direct != "" {
					directField = `"session_id":` + tc.direct + `,`
				}
				body := `{"model":"gpt-6",` + directField + `"client_metadata":{"session_id":"nested-owned-session"},"input":[{"role":"user","content":"independent"}]}`
				request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
					Header: make(http.Header), Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
				response, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Close()
				if response.Result.AccountID != tc.want || response.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("HTTP direct=%s unexpectedly selected nested owner: selected=%q want=%q", tc.name, response.Result.AccountID, tc.want)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{session: "nested-owned-session"}, router.now())
				if err != nil || owner != websocketQuotaOwnerB {
					t.Fatalf("shadowed session owner changed: owner=%q err=%v", owner, err)
				}
			})
		}
	}
}
