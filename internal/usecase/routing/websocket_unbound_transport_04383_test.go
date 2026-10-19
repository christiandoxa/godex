package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04383UnboundTurnFailuresRecoverOnlyFullHistory(t *testing.T) {
	for _, failure := range []string{"transport", "quota"} {
		for _, fullHistory := range []bool{false, true} {
			for _, persisted := range []bool{false, true} {
				t.Run(failure+"/"+map[bool]string{false: "delta", true: "history"}[fullHistory]+map[bool]string{false: "/memory", true: "/durable"}[persisted], func(t *testing.T) {
					failureResponse := &proxymodel.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")), WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true}}
					if failure == "quota" {
						failureResponse = websocketQuotaFailureResponse()
					}
					gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
						failureResponse,
						websocketCommittedTestResponse("resp-recovered"),
					}}
					config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA, Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
					if persisted {
						config.Bindings = routingrepository.NewStore(t.TempDir())
					}
					router, err := NewRouter(config)
					if err != nil {
						t.Fatal(err)
					}
					defer router.Close()
					if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{session: "session-a", previous: "resp-original"}, router.now()); err != nil {
						t.Fatal(err)
					}
					input := `[{"type":"function_call_output","call_id":"call-old","output":"done"}]`
					if fullHistory {
						input = `[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]`
					}
					request := websocketDispatchRequest(`{"type":"response.create","session_id":"session-a","client_metadata":{"x-codex-turn-state":"unknown-turn"},"input":`+input+`}`, 843307)
					request.Header.Set("x-codex-turn-state", "unknown-turn")
					forwarded, err := router.Forward(t.Context(), request)
					if err != nil {
						t.Fatal(err)
					}
					defer forwarded.Close()
					if fullHistory {
						if forwarded.Result.AccountID != websocketQuotaOwnerB || forwarded.Result.Failed || strings.Join(gateway.accounts, ",") != websocketQuotaOwnerA+","+websocketQuotaOwnerB {
							t.Fatalf("unbound full-history transport did not recover: accounts=%v failed=%t", gateway.accounts, forwarded.Result.Failed)
						}
						assertProdex04357TurnStateScrubbed(t, gateway.requests[1])
					} else {
						if strings.Join(gateway.accounts, ",") != websocketQuotaOwnerA || !forwarded.Result.Failed {
							t.Fatalf("unbound delta-only replayed: accounts=%v failed=%t", gateway.accounts, forwarded.Result.Failed)
						}
						if failure == "transport" && forwarded.Result.Response.StatusCode != http.StatusServiceUnavailable {
							t.Fatalf("transport lost terminal 503: %d", forwarded.Result.Response.StatusCode)
						}
						if failure == "quota" && (forwarded.Result.Response.PrecommitFailure == nil || forwarded.Result.Response.PrecommitFailure.Code != "insufficient_quota") {
							t.Fatal("quota error was not preserved")
						}
					}
					store := router.affinity
					if persisted {
						store = newAffinityStore()
						store.repository = config.Bindings
					}
					keys := affinityKeys{turn: "unknown-turn", session: "session-a", previous: "resp-original"}
					store.mu.Lock()
					if err := store.loadMissingLocked(t.Context(), keys, router.now()); err != nil {
						store.mu.Unlock()
						t.Fatal(err)
					}
					turn := store.values[affinityDigest("turn", keys.turn)].accountID
					session := store.values[affinityDigest("session", keys.session)].accountID
					store.mu.Unlock()
					wantSession := websocketQuotaOwnerA
					if fullHistory {
						wantSession = routingentity.ConflictAccountID
					}
					original, err := store.owner(t.Context(), affinityKeys{previous: "resp-original"}, router.now())
					if turn != "" || session != wantSession || original != websocketQuotaOwnerA || err != nil {
						t.Fatalf("unbound replay changed owners: turn=%q session=%q original=%q err=%v", turn, session, original, err)
					}
				})
			}
		}
	}
}
