package routing

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04383WebSocketRetryResetRequiresPreviousOwner(t *testing.T) {
	for _, failure := range []struct {
		name, code string
		status     int
	}{
		{"transport", "", http.StatusBadGateway},
		{"quota", "insufficient_quota", http.StatusTooManyRequests},
		{"rate", "rate_limit_exceeded", http.StatusTooManyRequests},
		{"overload", "server_is_overloaded", http.StatusServiceUnavailable},
	} {
		for _, owned := range []bool{false, true} {
			for _, durable := range []bool{false, true} {
				name := failure.name + "/" + map[bool]string{false: "unknown", true: "owned"}[owned] + map[bool]string{false: "/memory", true: "/durable"}[durable]
				t.Run(name, func(t *testing.T) {
					payload := `{"type":"error","status":` + strconv.Itoa(failure.status) + `,"error":{"code":"` + failure.code + `","message":"synthetic failure"}}`
					gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{{
						StatusCode: failure.status, Body: io.NopCloser(strings.NewReader(payload)),
						WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Transport: failure.name == "transport", Code: failure.code},
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
					} else if failure.name == "transport" {
						assertTerminalTransport50304383(t, forwarded.Result.Response)
					} else {
						got, readErr := io.ReadAll(forwarded.Result.Response.Body)
						if readErr != nil || string(got) != payload || forwarded.Result.Response.StatusCode != failure.status {
							t.Fatalf("unowned previous ID changed the upstream failure: status=%d body=%s err=%v", forwarded.Result.Response.StatusCode, got, readErr)
						}
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
}
