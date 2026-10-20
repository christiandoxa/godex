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

// Official 0.438.3 returns one local 503 WebSocket frame and zero model
// dispatches when a raw turn and its verified Compact lineage have conflicting
// owners. Selecting either profile would expose another account's history.
func TestProdex04383WebSocketCompactTurnOwnerConflictFailsClosed(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unsafe")),
					WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "should-not-exist"},
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
			for _, fixture := range []struct {
				owner string
				keys  affinityKeys
			}{
				{websocketQuotaOwnerA, affinityKeys{turn: "compact-turn"}},
				{websocketQuotaOwnerB, affinityKeys{turn: "__compact_turn_state__:compact-turn"}},
			} {
				if err := router.affinity.remember(t.Context(), fixture.owner, fixture.keys, router.now()); err != nil {
					t.Fatal(err)
				}
			}
			body := `{"type":"response.create","model":"gpt-6","x-codex-turn-state":"compact-turn","input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"next"}]}`
			request := websocketDispatchRequest(body, 843328)
			request.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
			forwarded, err := router.Forward(t.Context(), request)
			if forwarded != nil {
				_ = forwarded.Close()
			}
			var routeError *proxymodel.Error
			if !errors.As(err, &routeError) || routeError.StatusCode != http.StatusServiceUnavailable ||
				routeError.Message != upstreamTransportFailureMessage {
				t.Fatalf("owner conflict did not fail closed with official 503: err=%v attempts=%v", err, gateway.accounts)
			}
			if len(gateway.accounts) != 0 || len(gateway.requests) != 0 {
				t.Fatalf("conflicting Compact turn dispatched model: accounts=%v requests=%d", gateway.accounts, len(gateway.requests))
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			for _, fixture := range []struct {
				owner string
				keys  affinityKeys
			}{
				{websocketQuotaOwnerA, affinityKeys{turn: "compact-turn"}},
				{websocketQuotaOwnerB, affinityKeys{turn: "__compact_turn_state__:compact-turn"}},
			} {
				owner, err := store.owner(t.Context(), fixture.keys, router.now())
				if err != nil || owner != fixture.owner {
					t.Fatalf("conflicting Compact binding mutated: owner=%q want=%q err=%v", owner, fixture.owner, err)
				}
			}
		})
	}
}

func TestProdex04383WebSocketMatchingTurnAndCompactAliasOwnersStillDispatch(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{websocketCommittedTestResponse("resp-matching-owners")}}
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
			for _, turn := range []string{"compact-turn", "__compact_turn_state__:compact-turn"} {
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB,
					affinityKeys{turn: turn}, router.now()); err != nil {
					t.Fatal(err)
				}
			}
			body := `{"type":"response.create","x-codex-turn-state":"compact-turn","input":[{"type":"message","role":"user","content":"full history"}]}`
			request := websocketDispatchRequest(body, 843329)
			request.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
			forwarded, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer forwarded.Close()
			if forwarded.Result.AccountID != websocketQuotaOwnerB || len(gateway.accounts) != 1 ||
				gateway.accounts[0] != websocketQuotaOwnerB || string(gateway.requests[0].Body) != body {
				t.Fatalf("identical verified Compact owners were rejected or changed: owner=%q attempts=%v", forwarded.Result.AccountID, gateway.accounts)
			}
		})
	}
}
