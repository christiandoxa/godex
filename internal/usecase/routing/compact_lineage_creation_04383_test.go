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

// Tagged Prodex 0.438.3 remembers Compact session and turn-state lineage at
// successful HTTP Compact commitment. A raw response turn alone is not a
// substitute for these distinct durable aliases on later WebSocket followups.
func TestProdex04383HTTPCompactCommitCreatesTypedLineage(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name      string
			route     quotamodel.RouteKind
			turn      string
			wantAlias bool
			previous  bool
		}{
			{"compact-session-and-turn", quotamodel.RouteKindCompact, "generated-turn", true, false},
			{"compact-session-only", quotamodel.RouteKindCompact, "", true, false},
			{"compact-owned-previous-and-session", quotamodel.RouteKindCompact, "generated-turn", true, true},
			{"responses-do-not-create-compact-lineage", quotamodel.RouteKindResponses, "generated-turn", false, false},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					header := make(http.Header)
					if tc.turn != "" {
						header.Set("x-codex-turn-state", tc.turn)
					}
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: header,
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-compact","object":"response","output":[]}`))}
				}}
				config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerB,
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
				if tc.previous {
					if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
						affinityKeys{previous: "verified-previous"}, router.now()); err != nil {
						t.Fatal(err)
					}
				}
				path := "/backend-api/codex/responses"
				if tc.route == quotamodel.RouteKindCompact {
					path += "/compact"
				}
				body := `{"model":"gpt-6","session_id":"compact-session","input":[]}`
				if tc.previous {
					body = `{"model":"gpt-6","previous_response_id":"verified-previous","session_id":"compact-session","input":[]}`
				}
				request := proxymodel.Request{Method: http.MethodPost, Path: path, Header: make(http.Header),
					Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: tc.route},
				}
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != websocketQuotaOwnerB || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("Compact producer chose wrong owner/status: %s / %d", result.Result.AccountID, result.Result.Response.StatusCode)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for _, test := range []struct {
					name string
					keys affinityKeys
					want string
				}{
					{"direct session", affinityKeys{session: "compact-session"}, websocketQuotaOwnerB},
					{"compact session alias", affinityKeys{session: "__compact_session__:compact-session"}, map[bool]string{true: websocketQuotaOwnerB, false: ""}[tc.wantAlias]},
					{"compact turn alias", affinityKeys{turn: "__compact_turn_state__:generated-turn"}, map[bool]string{true: websocketQuotaOwnerB, false: ""}[tc.wantAlias && tc.turn != ""]},
				} {
					owner, err := store.owner(t.Context(), test.keys, router.now())
					if err != nil || owner != test.want {
						t.Fatalf("%s owner = %q, want %q, err=%v", test.name, owner, test.want, err)
					}
				}
			})
		}
	}
}
