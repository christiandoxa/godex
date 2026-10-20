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

// Prodex 0.438.3 validates response/session/turn hard binding components
// through RuntimeHardBindingIdentity::new: <= 1,024 UTF-8 bytes, no C0/C1
// controls, and at least one visible non-whitespace character.
func TestProdex04383HTTPHardBindingComponentLimits(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, value string
			valid       bool
		}{
			{"exact-1024", strings.Repeat("x", 1024), true},
			{"too-long-1025", strings.Repeat("x", 1025), false},
			{"control-c0", "resp\u0001control", false},
			{"control-c1", "resp\u0085control", false},
			{"valid-unicode", "resp-K-Ω", true},
			{"whitespace-only", "  ", false},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				encodedID, err := json.Marshal(tc.value)
				if err != nil {
					t.Fatal(err)
				}
				reply := `{"object":"response","id":` + string(encodedID) + `,"output":[]}`
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(reply))}
				}}
				cfg := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
					Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					cfg.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				result, err := router.Forward(t.Context(), proxymodel.Request{Method: http.MethodPost,
					Path: "/backend-api/codex/responses", Header: make(http.Header),
					Body:           []byte(`{"model":"gpt-6","input":[{"role":"user","content":"independent"}]}`),
					QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}})
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.Response.StatusCode != http.StatusOK || result.Result.AccountID != websocketQuotaOwnerA {
					t.Fatalf("wrong upstream for %s: owner=%q", tc.name, result.Result.AccountID)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = cfg.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{previous: tc.value}, router.now())
				want := ""
				if tc.valid {
					want = websocketQuotaOwnerA
				}
				if err != nil || owner != want {
					t.Fatalf("component %s owner=%q want=%q err=%v", tc.name, owner, want, err)
				}
			})
		}
	}
}

func TestProdex04383HTTPRequestedSessionComponentLimits(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, value string
			valid       bool
		}{
			{"exact-1024", strings.Repeat("s", 1024), true},
			{"too-long-1025", strings.Repeat("s", 1025), false},
			{"control-c0", "sess\x01bad", false},
			{"control-c1", "sess\u0085bad", false},
			{"valid-unicode", "session-Ω-K", true},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-valid-session","object":"response","output":[]}`))}
				}}
				cfg := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
					Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					cfg.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				encodedID, err := json.Marshal(tc.value)
				if err != nil {
					t.Fatal(err)
				}
				body := `{"model":"gpt-6","session_id":` + string(encodedID) + `,"input":[{"role":"user","content":"independent"}]}`
				result, err := router.Forward(t.Context(), proxymodel.Request{
					Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header),
					Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
				})
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != websocketQuotaOwnerA || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("session %s changed selected account/status: %q/%d", tc.name, result.Result.AccountID, result.Result.Response.StatusCode)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = cfg.Bindings
				}
				want := ""
				if tc.valid {
					want = websocketQuotaOwnerA
				}
				if owner, err := store.owner(t.Context(), affinityKeys{session: tc.value}, router.now()); err != nil || owner != want {
					t.Fatalf("request session %s owner=%q want=%q err=%v", tc.name, owner, want, err)
				}
			})
		}
	}
}
