package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// Official Prodex 0.438.3 inspects an owned previous response and the
// Compact-origin turn lineage together. Conflicting verified owners exhaust
// strict affinity locally with a 503, not a request on either account.
func TestProdex04383WebSocketPreviousResponseCompactLineageConflict(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, conflict := range []bool{false, true} {
			name := "memory"
			if durable {
				name = "durable"
			}
			if conflict {
				name += "/conflict"
			} else {
				name += "/same-owner"
			}
			t.Run(name, func(t *testing.T) {
				gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
					{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("unsafe")),
						WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp-unexpected"},
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
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA,
					affinityKeys{previous: "resp-main"}, router.now()); err != nil {
					t.Fatal(err)
				}
				aliasOwner := websocketQuotaOwnerA
				if conflict {
					aliasOwner = websocketQuotaOwnerB
				}
				if err := router.affinity.remember(t.Context(), aliasOwner,
					affinityKeys{turn: "__compact_turn_state__:compact-turn"}, router.now()); err != nil {
					t.Fatal(err)
				}
				body := `{"type":"response.create","model":"gpt-6","previous_response_id":"resp-main","x-codex-turn-state":"compact-turn","input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`
				request := websocketDispatchRequest(body, 843330)
				request.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
				result, err := router.Forward(t.Context(), request)
				if conflict {
					if result != nil {
						_ = result.Close()
					}
					var routeErr *proxymodel.Error
					if !errors.As(err, &routeErr) || routeErr.StatusCode != http.StatusServiceUnavailable ||
						routeErr.Message != upstreamTransportFailureMessage {
						t.Fatalf("conflicting owners were not rejected with pinned 503: error=%v attempts=%v", err, gateway.accounts)
					}
					if len(gateway.accounts) != 0 || len(gateway.requests) != 0 {
						t.Fatalf("conflicting previous/Compact lineage dispatched inference: accounts=%v", gateway.accounts)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					defer result.Close()
					if result.Result.AccountID != websocketQuotaOwnerA || len(gateway.accounts) != 1 ||
						gateway.accounts[0] != websocketQuotaOwnerA || string(gateway.requests[0].Body) != body {
						t.Fatalf("same-owner previous/Compact lineage changed: owner=%q attempts=%v", result.Result.AccountID, gateway.accounts)
					}
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for _, tc := range []struct {
					label string
					keys  affinityKeys
					want  string
				}{
					{"previous", affinityKeys{previous: "resp-main"}, websocketQuotaOwnerA},
					{"Compact turn", affinityKeys{turn: "__compact_turn_state__:compact-turn"}, aliasOwner},
				} {
					owner, err := store.owner(t.Context(), tc.keys, router.now())
					if err != nil || owner != tc.want {
						t.Fatalf("%s mutated: owner=%q want=%q err=%v", tc.label, owner, tc.want, err)
					}
				}
			})
		}
	}
}
