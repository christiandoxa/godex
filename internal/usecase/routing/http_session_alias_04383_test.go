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

// Prodex 0.438.3 looks for direct session_id or client_metadata.session_id.
// Arbitrary conversation/thread fields are payload, never request affinity.
func TestProdex04383HTTPSessionAliasesDoNotStealOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, fields string
			owner        string
		}{
			{"conversation-alias", `"conversation_id":"shadow-session",`, websocketQuotaOwnerA},
			{"thread-alias", `"thread_id":"shadow-session",`, websocketQuotaOwnerA},
			{"unrelated-aliases", `"conversation_id":"shadow-session","thread_id":"shadow-session",`, websocketQuotaOwnerA},
			{"nested", `"client_metadata":{"session_id":"shadow-session"},`, websocketQuotaOwnerB},
			{"direct", `"session_id":"shadow-session",`, websocketQuotaOwnerB},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-http-session-alias","object":"response","output":[]}`))}
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
				body := `{"model":"gpt-6",` + tc.fields + `"input":[{"role":"user","content":"independent"}]}`
				request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
					Header: make(http.Header), Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != tc.owner || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("HTTP %s selected=%q want=%q", tc.name, result.Result.AccountID, tc.owner)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{session: "shadow-session"}, router.now())
				if err != nil || owner != websocketQuotaOwnerB {
					t.Fatalf("verified original session owner changed: owner=%q err=%v", owner, err)
				}
			})
		}
	}
}

func TestProdex04383WebSocketSessionAliasesAreNotContinuationKeys(t *testing.T) {
	for _, tc := range []struct {
		name, fields, want string
	}{
		{"conversation", `"conversation_id":"shadow-session",`, ""},
		{"thread", `"thread_id":"shadow-session",`, ""},
		{"direct", `"session_id":"shadow-session",`, "shadow-session"},
		{"nested", `"client_metadata":{"session_id":"shadow-session"},`, "shadow-session"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"type":"response.create",` + tc.fields + `"input":[]}`)
			request := proxymodel.Request{Body: body, Header: make(http.Header), WebSocketMessage: true}
			if got := requestRoutingAffinity(request).session; got != tc.want {
				t.Fatalf("WebSocket %s session affinity=%q want=%q", tc.name, got, tc.want)
			}
		})
	}
}
