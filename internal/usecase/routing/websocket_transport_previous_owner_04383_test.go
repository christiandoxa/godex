package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04383WebSocketTransportResetRequiresPreviousOwner(t *testing.T) {
	for _, owned := range []bool{false, true} {
		for _, durable := range []bool{false, true} {
			name := map[bool]string{false: "unknown", true: "owned"}[owned] + map[bool]string{false: "/memory", true: "/durable"}[durable]
			t.Run(name, func(t *testing.T) {
				gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{{
					StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")),
					WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true},
				}}}
				config := Config{Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				keys := affinityKeys{turn: "turn-a", session: "session-a"}
				if owned {
					keys.previous = "resp-input"
				}
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, keys, router.now()); err != nil {
					t.Fatal(err)
				}
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{previous: "resp-unrelated"}, router.now()); err != nil {
					t.Fatal(err)
				}
				const body = `{"type":"response.create","previous_response_id":"resp-input","session_id":"session-a","client_metadata":{"x-codex-turn-state":"turn-a","sequence":18446744073709551615},"input":[{"role":"user","content":"first","sequence":9007199254740993},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`
				request := websocketDispatchRequest(body, 843308)
				request.Header.Set("x-codex-turn-state", "turn-a")
				forwarded, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer forwarded.Close()
				if len(gateway.accounts) != 1 || gateway.accounts[0] != websocketQuotaOwnerA || !forwarded.Result.Failed || string(gateway.requests[0].Body) != body || gateway.requests[0].Header.Get("x-codex-turn-state") != "turn-a" {
					t.Fatalf("transport changed owner, opaque request or replayed: accounts=%v failed=%t", gateway.accounts, forwarded.Result.Failed)
				}
				if owned {
					assertProdex04355FullContextSignal(t, forwarded, websocketQuotaOwnerA)
				} else {
					assertTerminalTransport50304383(t, forwarded.Result.Response)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for label, key := range map[string]affinityKeys{
					"previous": {previous: "resp-input"}, "turn": {turn: keys.turn},
					"session": {session: keys.session}, "unrelated": {previous: "resp-unrelated"},
				} {
					want := websocketQuotaOwnerA
					if label == "previous" || (owned && label == "turn") {
						want = ""
					}
					owner, err := store.owner(t.Context(), key, router.now())
					if err != nil || owner != want {
						t.Fatalf("%s owner=%q err=%v, want %q", label, owner, err, want)
					}
				}
			})
		}
	}
}
