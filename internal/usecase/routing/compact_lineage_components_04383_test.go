package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// Prodex 0.438.3's tagged RuntimeHardBindingIdentity filter uses the Mojo
// component boundary: <=1024 UTF-8 bytes and no C0/C1 controls. Compact's
// typed lineage aliases must never mint an owner for an invalid component.
func TestProdex04383HTTPCompactLineageRejectsInvalidIdentityComponents(t *testing.T) {
	for _, tc := range []struct {
		name, session, turn   string
		wantSession, wantTurn bool
	}{
		{"boundary-turn-1024", "compact-session", strings.Repeat("t", 1024), true, true},
		{"oversized-turn-1025", "compact-session", strings.Repeat("t", 1025), true, false},
		{"embedded-nul-turn", "compact-session", "good\x00bad", true, false},
		{"embedded-tab-turn", "compact-session", "good\tbad", true, false},
		{"embedded-del-turn", "compact-session", "good\x7fbad", true, false},
		{"embedded-c1-turn", "compact-session", "good\u0085bad", true, false},
		{"invalid-utf8-turn", "compact-session", "good\xffbad", true, false},
		{"boundary-session-1024-utf8", strings.Repeat("é", 512), "turn-good", true, true},
		{"oversized-session-1025", strings.Repeat("s", 1025), "turn-good", false, true},
		{"embedded-nul-session", "good\x00session", "turn-good", false, true},
		{"embedded-c1-session", "good\u0085session", "turn-good", false, true},
	} {
		for _, durable := range []bool{false, true} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					header := make(http.Header)
					header.Set("x-codex-turn-state", tc.turn)
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: header,
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-typed-component","object":"response","output":[]}`))}
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
				body, err := json.Marshal(map[string]any{
					"model": "gpt-6", "session_id": tc.session, "input": []any{},
				})
				if err != nil {
					t.Fatal(err)
				}
				request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses/compact",
					Header: make(http.Header), Body: body, QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindCompact}}
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != websocketQuotaOwnerB {
					t.Fatalf("HTTP Compact owner changed: %q", result.Result.AccountID)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for _, binding := range []struct {
					name       string
					keys       affinityKeys
					shouldBind bool
				}{
					{"compact session", affinityKeys{session: "__compact_session__:" + tc.session}, tc.wantSession},
					{"compact turn", affinityKeys{turn: "__compact_turn_state__:" + tc.turn}, tc.wantTurn},
				} {
					owner, err := store.owner(t.Context(), binding.keys, router.now())
					if err != nil {
						t.Fatal(err)
					}
					want := ""
					if binding.shouldBind {
						want = websocketQuotaOwnerB
					}
					if owner != want {
						t.Fatalf("%s owner=%q want=%q", binding.name, owner, want)
					}
				}
			})
		}
	}
}
