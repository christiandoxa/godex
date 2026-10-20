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

// Prodex 0.438.3 runtime_request_thread_id supports the canonical thread-id
// header for trace metadata, but does not recognize x-codex-thread-id.
// The unsupported spelling cannot create a foreign hard routing owner.
func TestProdex04383UnsupportedHTTPThreadHeaderCannotCreateAffinity(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
				return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{"object":"response","output":[]}`))}
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
				affinityKeys{previous: "previous-second"}, router.now()); err != nil {
				t.Fatal(err)
			}
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
				affinityKeys{previous: "previous-main"}, router.now()); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name, body, thread, want string
			}{
				{"previous-second", `{"previous_response_id":"previous-second","input":[]}`, "trace-only", websocketQuotaOwnerB},
				{"restore-main", `{"previous_response_id":"previous-main","input":[]}`, "", websocketQuotaOwnerA},
				{"unsupported-thread-reuse", `{"model":"gpt-6","input":[]}`, "trace-only", websocketQuotaOwnerA},
			} {
				header := make(http.Header)
				if tc.thread != "" {
					header.Set("x-codex-thread-id", tc.thread)
				}
				result, err := router.Forward(t.Context(), proxymodel.Request{
					Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: header,
					Body: []byte(tc.body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
				})
				if err != nil {
					t.Fatalf("%s: %v", tc.name, err)
				}
				if result.Result.AccountID != tc.want || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("%s selected=%q want=%q", tc.name, result.Result.AccountID, tc.want)
				}
				_ = result.Close()
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			if owner, err := store.owner(t.Context(), affinityKeys{thread: "trace-only"}, router.now()); err != nil || owner != "" {
				t.Fatalf("unsupported thread header created binding: owner=%q err=%v", owner, err)
			}
		})
	}
}
