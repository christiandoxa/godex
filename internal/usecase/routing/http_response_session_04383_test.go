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

// Tagged Prodex 0.438.3 remembers session IDs on successful Responses from
// the REQUEST semantics, not untrusted session-like response payload fields.
func TestProdex04383ResponsesDoNotBindUnrequestedResponseSession(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, payload, responseSessionHeader, requestSession string
			wantSession                                          bool
		}{
			{"top-level-session", `{"id":"resp-spoof","object":"response","session_id":"spoof-session","output":[]}`, "", "", false},
			{"response-object-session", `{"id":"resp-spoof","object":"response","response":{"session_id":"spoof-session"},"output":[]}`, "", "", false},
			{"data-session", `{"id":"resp-spoof","object":"response","data":{"session_id":"spoof-session"},"output":[]}`, "", "", false},
			{"conversation-alias", `{"id":"resp-spoof","object":"response","conversation_id":"spoof-session","output":[]}`, "", "", false},
			{"thread-alias", `{"id":"resp-spoof","object":"response","thread_id":"spoof-session","output":[]}`, "", "", false},
			{"response-session-header", `{"id":"resp-spoof","object":"response","output":[]}`, "spoof-session", "", false},
			{"request-session", `{"id":"resp-spoof","object":"response","output":[]}`, "", "spoof-session", true},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					header := http.Header{"Content-Type": []string{"application/json"}}
					if tc.responseSessionHeader != "" {
						header.Set("session-id", tc.responseSessionHeader)
					}
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(tc.payload))}
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
				body := `{"model":"gpt-6","input":[{"role":"user","content":"no implicit session"}]}`
				if tc.requestSession != "" {
					body = `{"model":"gpt-6","session_id":"spoof-session","input":[{"role":"user","content":"explicit session"}]}`
				}
				result, err := router.Forward(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
					Header: make(http.Header), Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}})
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != websocketQuotaOwnerA || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("unexpected result: owner=%q status=%d", result.Result.AccountID, result.Result.Response.StatusCode)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{session: "spoof-session"}, router.now())
				if err != nil {
					t.Fatal(err)
				}
				wantOwner := ""
				if tc.wantSession {
					wantOwner = websocketQuotaOwnerA
				}
				if owner != wantOwner {
					t.Fatalf("response-only session unexpectedly bound: owner=%q want=%q", owner, wantOwner)
				}
				responseOwner, err := store.owner(t.Context(), affinityKeys{previous: "resp-spoof"}, router.now())
				if err != nil || responseOwner != websocketQuotaOwnerA {
					t.Fatalf("valid response owner missing: owner=%q err=%v", responseOwner, err)
				}
			})
		}
	}
}
