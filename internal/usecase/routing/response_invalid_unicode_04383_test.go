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

// Prodex's extract_runtime_response_ids_from_body_bytes parses the *whole*
// response with serde_json before admitting a continuation identifier.
func TestProdex04383InvalidUnicodeResponseCannotCreateVerifiedIDs(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, payload string
			wantOwner     bool
		}{
			{"lone-high", `{"object":"response","id":"resp-opaque","note":"\ud800","output":[]}`, false},
			{"lone-low", `{"object":"response","id":"resp-opaque","note":"\udc00","output":[]}`, false},
			{"invalid-key", `{"object":"response","id":"resp-opaque","note\ud800":"x","output":[]}`, false},
			{"valid-pair", `{"object":"response","id":"resp-opaque","note":"\ud83d\ude00","output":[]}`, true},
			{"escaped-slash", `{"object":"response","id":"resp-opaque","note":"\\ud800","output":[]}`, true},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
						Body: io.NopCloser(strings.NewReader(tc.payload))}
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
				exchange, err := router.Forward(t.Context(), proxymodel.Request{
					Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header),
					Body:           []byte(`{"model":"gpt-6","input":[{"role":"user","content":"independent"}]}`),
					QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
				})
				if err != nil {
					t.Fatal(err)
				}
				defer exchange.Close()
				if exchange.Result.AccountID != websocketQuotaOwnerA || exchange.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("unexpected HTTP result owner=%q status=%d", exchange.Result.AccountID, exchange.Result.Response.StatusCode)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{previous: "resp-opaque"}, router.now())
				want := ""
				if tc.wantOwner {
					want = websocketQuotaOwnerA
				}
				if err != nil || owner != want {
					t.Fatalf("response %s minted incorrect lineage: got=%q want=%q err=%v", tc.name, owner, want, err)
				}
			})
		}
	}
}

func TestProdex04383InvalidUnicodeSSEEventCannotCreateVerifiedIDs(t *testing.T) {
	router, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	for _, tc := range []struct {
		id, note string
		valid    bool
	}{
		{"resp-invalid", `"\ud800"`, false},
		{"resp-valid", `"\ud83d\ude00"`, true},
	} {
		event := "data: " + `{"type":"response.completed","response":{"id":"` + tc.id + `"},"note":` + tc.note + "}\n\n"
		if err := router.Observe(t.Context(), websocketQuotaOwnerA, nil, []byte(event), true); err != nil {
			t.Fatal(err)
		}
		owner, err := router.affinity.owner(t.Context(), affinityKeys{previous: tc.id}, router.now())
		want := ""
		if tc.valid {
			want = websocketQuotaOwnerA
		}
		if err != nil || owner != want {
			t.Fatalf("SSE event id=%s gained improper owner=%q want=%q err=%v", tc.id, owner, want, err)
		}
	}
}
