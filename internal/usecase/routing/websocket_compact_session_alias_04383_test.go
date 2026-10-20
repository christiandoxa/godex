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

// Tagged 0.438.3 resolves Compact session lineage separately from the normal
// session key even while the inbound message retains RouteKindWebSocket. The
// existing direct owner and previous-response owner must keep precedence.
func TestProdex04383WebSocketCompactSessionAliasSelection(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, body                      string
			header, alias, direct, previous bool
			want                            string
		}{
			{"header-alias", `{"type":"response.create","input":[{"role":"user","content":"hello"}]}`, true, true, false, false, websocketQuotaOwnerB},
			{"body-alias", `{"type":"response.create","session_id":"session-alias","input":[{"role":"user","content":"hello"}]}`, false, true, false, false, websocketQuotaOwnerB},
			{"header-unbound", `{"type":"response.create","input":[{"role":"user","content":"hello"}]}`, true, false, false, false, websocketQuotaOwnerA},
			{"body-unbound", `{"type":"response.create","session_id":"session-alias","input":[{"role":"user","content":"hello"}]}`, false, false, false, false, websocketQuotaOwnerA},
			{"direct-owner-precedence", `{"type":"response.create","input":[{"role":"user","content":"hello"}]}`, true, true, true, false, websocketQuotaOwnerA},
			{"previous-owner-precedence", `{"type":"response.create","previous_response_id":"resp-main","input":[{"role":"user","content":"hello"}]}`, true, true, false, true, websocketQuotaOwnerA},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
					{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("committed")),
						WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp-compact-session"},
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
				if tc.alias {
					if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
						affinityKeys{session: "__compact_session__:session-alias"}, router.now()); err != nil {
						t.Fatal(err)
					}
				}
				if tc.direct {
					if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
						affinityKeys{session: "session-alias"}, router.now()); err != nil {
						t.Fatal(err)
					}
				}
				if tc.previous {
					if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
						affinityKeys{previous: "resp-main"}, router.now()); err != nil {
						t.Fatal(err)
					}
				}
				request := websocketDispatchRequest(tc.body, 843327)
				request.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
				if tc.header {
					request.Header.Set("session_id", "session-alias")
				}
				forwarded, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer forwarded.Close()
				if forwarded.Result.AccountID != tc.want || len(gateway.accounts) != 1 || gateway.accounts[0] != tc.want {
					t.Fatalf("session alias selected wrong owner: got=%q want=%q attempts=%v", forwarded.Result.AccountID, tc.want, gateway.accounts)
				}
				if string(gateway.requests[0].Body) != tc.body || gateway.requests[0].Header.Get("x-codex-turn-state") != "" {
					t.Fatal("session-only WebSocket changed opaque input or invented turn state")
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				wantAlias := ""
				if tc.alias {
					wantAlias = websocketQuotaOwnerB
				}
				alias, err := store.owner(t.Context(), affinityKeys{session: "__compact_session__:session-alias"}, router.now())
				if err != nil || alias != wantAlias {
					t.Fatalf("durable Compact alias changed: got=%q want=%q err=%v", alias, wantAlias, err)
				}
			})
		}
	}
}
