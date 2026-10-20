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

// Prodex 0.438.3's runtime_request_semantic_plan reads the request's
// previous_response_id only from the direct JSON member. A nested response
// object must not silently adopt another profile's hard previous owner.
func TestProdex04383HTTPNestedPreviousCannotImpersonateHardOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, direct string
			want         string
		}{
			{"missing-direct", "", websocketQuotaOwnerA},
			{"direct-null", `null`, websocketQuotaOwnerA},
			{"direct-blank", `"  "`, websocketQuotaOwnerA},
			{"direct-valid", `"resp-nested"`, websocketQuotaOwnerB},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-http-nested-previous","object":"response","output":[]}`))}
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
					affinityKeys{previous: "resp-nested"}, router.now()); err != nil {
					t.Fatal(err)
				}
				directField := ""
				if tc.direct != "" {
					directField = `"previous_response_id":` + tc.direct + `,`
				}
				body := `{"model":"gpt-6",` + directField + `"response":{"previous_response_id":"resp-nested"},"input":[{"role":"user","content":"independent"}]}`
				request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
					Header: make(http.Header), Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != tc.want || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("HTTP previous %s adopted nested hard owner: got=%q want=%q", tc.name, result.Result.AccountID, tc.want)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{previous: "resp-nested"}, router.now())
				if err != nil || owner != websocketQuotaOwnerB {
					t.Fatalf("nested previous owner changed: owner=%q err=%v", owner, err)
				}
			})
		}
	}
}
