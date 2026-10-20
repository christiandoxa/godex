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

// Tagged HTTP Responses selects turn affinity from the header only.
// Body turn-state metadata remains opaque on HTTP, including its nested form.
func TestProdex04383HTTPDirectTurnShadowsNestedMetadata(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, direct string
			want         string
			header       bool
		}{
			{"null", `null`, websocketQuotaOwnerA, false},
			{"empty", `""`, websocketQuotaOwnerA, false},
			{"blank", `"  "`, websocketQuotaOwnerA, false},
			{"boolean", `false`, websocketQuotaOwnerA, false},
			{"number", `123`, websocketQuotaOwnerA, false},
			{"object", `{"turn":"wrong"}`, websocketQuotaOwnerA, false},
			{"missing", ``, websocketQuotaOwnerA, false},
			{"header", `null`, websocketQuotaOwnerB, true},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-http-turn-shadow","object":"response","output":[]}`))}
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
					affinityKeys{turn: "nested-owned-turn"}, router.now()); err != nil {
					t.Fatal(err)
				}
				directField := ""
				if tc.direct != "" {
					directField = `"x-codex-turn-state":` + tc.direct + `,`
				}
				body := `{"model":"gpt-6",` + directField + `"client_metadata":{"x-codex-turn-state":"nested-owned-turn"},"input":[{"role":"user","content":"independent"}]}`
				request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
					Header: make(http.Header), Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
				if tc.header {
					request.Header.Set("x-codex-turn-state", "nested-owned-turn")
				}
				response, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Close()
				if response.Result.AccountID != tc.want || response.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("HTTP turn direct=%s selected=%q want=%q", tc.name, response.Result.AccountID, tc.want)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{turn: "nested-owned-turn"}, router.now())
				if err != nil || owner != websocketQuotaOwnerB {
					t.Fatalf("shadowed turn owner changed: owner=%q err=%v", owner, err)
				}
			})
		}
	}
}
